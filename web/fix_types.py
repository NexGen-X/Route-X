import re

path = 'src/pages/Providers.spec.ts'
with open(path, 'r') as f:
    content = f.read()

content = content.replace("const listWithHoles as unknown as Provider[] =", "const listWithHoles: unknown[] =")

with open(path, 'w') as f:
    f.write(content)
