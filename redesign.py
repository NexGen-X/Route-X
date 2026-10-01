import os
import re

def update_file(path, replacements):
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    
    orig = content
    for pattern, repl in replacements:
        content = re.sub(pattern, repl, content)
        
    if content != orig:
        with open(path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Updated {path}")

# Regex replacements for common and pages
common_replacements = {
    'Button.tsx': [
        (r'rounded-[\w-]+', 'rounded-full'),
        (r'className="([^"]*)"', r'className="\1 min-h-[48px] active:scale-[0.97] hover:shadow-[var(--surface-glow)]"')
    ],
    'Card.tsx': [
        (r'bg-bg-surface(-[\d]+)?', 'bg-bg-surface/40 backdrop-blur-md'),
        (r'border border-border', 'ring-1 ring-white/5'),
        (r'className="([^"]*)"', r'className="\1 hover:-translate-y-0.5 hover:shadow-xl"')
    ],
    'Sidebar.tsx': [
        (r'border-r border-border', ''),
        (r'(text-accent.*)bg-bg-surface-2', r'\1bg-bg-surface-2 shadow-[var(--surface-glow)]')
    ],
    'Badge.tsx': [
        (r'rounded(-[\w]+)?', 'rounded-full'),
        (r'px-2 py-0.5', 'px-3 py-1 md:px-2 md:py-0.5')
    ],
    'Modal.tsx': [
        (r'max-w-[\w-]+ w-full', r'w-full h-[100dvh] md:h-auto md:max-w-lg md:rounded-2xl'),
        (r'rounded-[\w-]+', '')
    ],
    'PageHeader.tsx': [
        (r'text-[\w]+ font-semibold', 'text-xl font-bold')
    ],
}

base_dir = '/root/Route-X/web/src'

for root_dir, dirs, files in os.walk(base_dir):
    for file in files:
        if not file.endswith('.tsx') and not file.endswith('.ts'):
            continue
        
        path = os.path.join(root_dir, file)
        with open(path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        orig = content
        
        # Zero any policy
        content = re.sub(r':\s*any\b', ': unknown', content)
        content = re.sub(r'<\s*any\s*>', '<unknown>', content)
        
        # Grid updates
        if 'grid-cols-' in content:
            content = re.sub(r'grid-cols-[\w-]+', 'grid-cols-1 md:grid-cols-2 lg:grid-cols-4', content)
            
        # Shimmer effect for skeleton
        if 'animate-pulse' in content:
            content = content.replace('animate-pulse', 'animate-[shimmer_1.5s_infinite] bg-gradient-to-r from-bg-surface-2 via-bg-surface-3 to-bg-surface-2 bg-[length:400%_100%]')
        
        # Apply specific replacements
        if file in common_replacements:
            for pattern, repl in common_replacements[file]:
                content = re.sub(pattern, repl, content)
                
        if content != orig:
            with open(path, 'w', encoding='utf-8') as f:
                f.write(content)
            print(f"Updated {path}")
