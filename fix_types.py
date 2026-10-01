import re

path = 'web/src/context/AuthContext.tsx'
with open(path, 'r') as f:
    content = f.read()

content = content.replace("const p: Principal = res.principal || res;", "const p: Principal = (res as Record<string, unknown>).principal as Principal || (res as Principal);")
content = content.replace("setCsrfToken((res as Record<string, unknown>).csrf_token);", "setCsrfToken((res as Record<string, unknown>).csrf_token as string);")

with open(path, 'w') as f:
    f.write(content)

path = 'web/src/pages/Observability.tsx'
with open(path, 'r') as f:
    content = f.read()
    
content = content.replace("const formatTooltipValue = (value: unknown) => {", "const formatTooltipValue = (value: number) => {")

with open(path, 'w') as f:
    f.write(content)
    
path = 'web/src/pages/Providers.spec.ts'
with open(path, 'r') as f:
    content = f.read()

content = content.replace("null as unknown", "null as unknown as Provider[]")
content = content.replace("undefined as unknown", "undefined as unknown as Provider[]")
content = content.replace("listWithHoles", "listWithHoles as unknown as Provider[]")
content = content.replace("'invalid' as unknown", "'invalid' as unknown as Provider")

with open(path, 'w') as f:
    f.write(content)
