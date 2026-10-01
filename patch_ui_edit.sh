sed -i '/setIsSubmitting(true);/i \
    if (editForm.proxy_url.trim()) {\
      const ssrfError = validateProxyUrl(editForm.proxy_url.trim());\
      if (ssrfError) {\
        toast.error(ssrfError, "URL Proxy Ditolak");\
        return;\
      }\
    }\
' web/src/components/egress/EditEgressDrawer.tsx
