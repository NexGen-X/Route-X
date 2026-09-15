package admin

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/httpx"
)

// getBackupStatus mengembalikan ringkasan statistik ukuran database dan info berkas cadangan lokal.
func (h *Handlers) getBackupStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var dbName, dbSize string
	var dbBytes int64
	var totalTables int

	row := h.pool.QueryRow(ctx, `
		SELECT 
			current_database(),
			pg_size_pretty(pg_database_size(current_database())),
			pg_database_size(current_database()),
			(SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public')
	`)
	if err := row.Scan(&dbName, &dbSize, &dbBytes, &totalTables); err != nil {
		mapRepoError(w, r, err, "status database")
		return
	}

	var modelsCount, providersCount, rulesCount, apiKeysCount int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM models`).Scan(&modelsCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM providers`).Scan(&providersCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM routing_rules`).Scan(&rulesCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM api_keys`).Scan(&apiKeysCount)

	status := BackupStatusDTO{
		DatabaseName:      dbName,
		DatabaseSize:      dbSize,
		DatabaseBytes:     dbBytes,
		TotalTables:       totalTables,
		ModelsCount:       modelsCount,
		ProvidersCount:    providersCount,
		RoutingRulesCount: rulesCount,
		APIKeysCount:      apiKeysCount,
	}

	// Periksa berkas cadangan terakhir di /var/backups/routex atau /var/backups bila ada
	backupDirs := []string{"/var/backups/routex", "/var/backups"}
	var latestFile os.FileInfo
	for _, dir := range backupDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), "routex") &&
					(strings.HasSuffix(entry.Name(), ".sql.gz") || strings.HasSuffix(entry.Name(), ".sql") || strings.HasSuffix(entry.Name(), ".dump")) {
					if info, err := entry.Info(); err == nil {
						if latestFile == nil || info.ModTime().After(latestFile.ModTime()) {
							latestFile = info
						}
					}
				}
			}
		}
	}
	if latestFile != nil {
		sizeStr := fmt.Sprintf("%.1f KB", float64(latestFile.Size())/1024)
		if latestFile.Size() > 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(latestFile.Size())/(1024*1024))
		}
		status.LastServerBackup = &ServerBackupInfo{
			FileName:  latestFile.Name(),
			FileSize:  sizeStr,
			CreatedAt: latestFile.ModTime(),
		}
	}

	_ = h.respond(w, r, http.StatusOK, status)
}

// cleanConnStringAndSchema membersihkan URI koneksi database dari parameter yang tidak didukung libpq/pg_dump (seperti search_path)
// dan mengekstrak nama skema jika ditentukan.
func cleanConnStringAndSchema(rawConn string) (string, string) {
	u, err := url.Parse(rawConn)
	if err != nil {
		return rawConn, ""
	}
	q := u.Query()
	schema := q.Get("search_path")
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String(), schema
}

// exportDatabaseSQL mengalirkan dump SQL database terkompresi (.sql.gz) atau teks polos (.sql) langsung ke klien.
func (h *Handlers) exportDatabaseSQL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "sql.gz"
	}

	connStr, schema := cleanConnStringAndSchema(h.pool.Config().ConnString())
	timestamp := time.Now().UTC().Format("20060102_150405")

	baseArgs := []string{"--clean", "--if-exists", "--no-owner", "--no-privileges"}
	if schema != "" {
		baseArgs = append(baseArgs, "-n", schema)
	}
	baseArgs = append(baseArgs, connStr)

	if format == "sql" {
		fileName := fmt.Sprintf("routex_backup_%s.sql", timestamp)
		w.Header().Set("Content-Type", "application/sql; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		w.Header().Set("Cache-Control", "no-store")

		cmd := exec.CommandContext(ctx, "pg_dump", baseArgs...)
		cmd.Stdout = w
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			h.logger.ErrorContext(ctx, "gagal mengekspor database sql", slog.String("error", stderr.String()), slog.Any("err", err))
			return
		}
	} else {
		fileName := fmt.Sprintf("routex_backup_%s.sql.gz", timestamp)
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		w.Header().Set("Cache-Control", "no-store")

		cmd := exec.CommandContext(ctx, "pg_dump", baseArgs...)
		gw := gzip.NewWriter(w)
		cmd.Stdout = gw
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			h.logger.ErrorContext(ctx, "gagal mengekspor database sql.gz", slog.String("error", stderr.String()), slog.Any("err", err))
			return
		}
		_ = gw.Close()
	}

	h.writeAudit(ctx, r, "export", "database_backup", "all", map[string]any{"format": format})
}

// restoreDatabaseSQL memulihkan skema dan data database dari berkas SQL yang diunggah (.sql atau .sql.gz).
func (h *Handlers) restoreDatabaseSQL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Batasi ukuran request body hingga 50 MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httpx.BadRequest(w, r, "file_too_large", "ukuran berkas backup melebihi batas maksimal 50 MB")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, r, "missing_file", "berkas cadangan 'file' (.sql atau .sql.gz) wajib diunggah")
		return
	}
	defer file.Close()

	lowerName := strings.ToLower(header.Filename)
	if !strings.HasSuffix(lowerName, ".sql") && !strings.HasSuffix(lowerName, ".sql.gz") {
		httpx.BadRequest(w, r, "invalid_file_extension", "ekstensi berkas cadangan harus berupa .sql atau .sql.gz")
		return
	}

	// Deteksi kompresi gzip lewat magic bytes (0x1f, 0x8b)
	headerBytes := make([]byte, 2)
	n, _ := file.Read(headerBytes)
	if n < 2 {
		httpx.BadRequest(w, r, "invalid_file", "berkas cadangan kosong atau rusak")
		return
	}

	var sqlReader io.Reader
	if headerBytes[0] == 0x1f && headerBytes[1] == 0x8b {
		// Berkas terkompresi gzip
		fullReader := io.MultiReader(bytes.NewReader(headerBytes[:n]), file)
		gzReader, err := gzip.NewReader(fullReader)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_gzip", fmt.Sprintf("arsip gzip rusak: %v", err))
			return
		}
		defer gzReader.Close()
		sqlReader = gzReader
	} else {
		// Berkas SQL teks polos
		sqlReader = io.MultiReader(bytes.NewReader(headerBytes[:n]), file)
	}

	connStr, schema := cleanConnStringAndSchema(h.pool.Config().ConnString())
	psqlArgs := []string{"-v", "ON_ERROR_STOP=1"}
	if schema != "" {
		psqlArgs = append(psqlArgs, "-c", fmt.Sprintf("SET search_path TO %s;", schema))
	}
	psqlArgs = append(psqlArgs, connStr)

	// Eksekusi psql dengan ON_ERROR_STOP=1
	cmd := exec.CommandContext(ctx, "psql", psqlArgs...)
	cmd.Stdin = sqlReader
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		h.logger.ErrorContext(ctx, "gagal memulihkan database dari backup sql",
			slog.String("filename", header.Filename),
			slog.String("stderr", errMsg),
		)
		httpx.BadRequest(w, r, "restore_failed", fmt.Sprintf("Gagal memulihkan database: %s", errMsg))
		return
	}

	h.writeAudit(ctx, r, "restore", "database_backup", "all", map[string]any{
		"filename": header.Filename,
		"size":     header.Size,
	})

	_ = h.respond(w, r, http.StatusOK, StatusResponse{
		Status: "success",
	})
}
