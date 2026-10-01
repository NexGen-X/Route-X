package main

import (
	"fmt"
	"golang.org/x/crypto/argon2"
	"crypto/rand"
	"encoding/base64"
)

func main() {
	password := "admin"
	salt := make([]byte, 16)
	rand.Read(salt)
	
	// Argon2id config matching standard parameters (time=1, memory=64MB, threads=4, keyLen=32)
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	
	// Format as PHC string
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	
	phc := fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", b64Salt, b64Hash)
	fmt.Println(phc)
}
