sed -i '/setIsSubmitting(true);/i \
    const ssrfError = validateProxyUrl(formData.proxy_url);\
    if (ssrfError) {\
      toast.error(ssrfError, "URL Proxy Ditolak");\
      return;\
    }\
' web/src/components/egress/CreateEgressDrawer.tsx
