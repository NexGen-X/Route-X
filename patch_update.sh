sed -i '/var params upstream.UpdateEgressParams/i \
	if req.ProxyURL != "" {\
		if err := security.ValidateProxyURL(req.ProxyURL, h.ssrfPolicy()); err != nil {\
			httpx.BadRequest(w, r, "invalid_proxy_url", err.Error())\
			return\
		}\
	}\
' internal/admin/upstreams.go
