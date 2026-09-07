const fs = require('fs');
let code = fs.readFileSync('web/src/components/common/CommandPalette.tsx', 'utf8');

// Ensure role="dialog" and aria-modal on the overlay
code = code.replace(
    '<div className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] sm:pt-[15vh]">',
    '<div className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] sm:pt-[15vh]" role="dialog" aria-modal="true" aria-label="Command Palette">'
);

// Search input aria-activedescendant
code = code.replace(
    'placeholder="Ketik perintah atau cari..."',
    'placeholder="Ketik perintah atau cari..."\n            aria-label="Cari perintah"\n            aria-activedescendant={results.length > 0 ? `cmd-opt-${selectedIndex}` : undefined}'
);

// Options ID
code = code.replace(
    'key={`${group.id}-${item.id}`}',
    'key={`${group.id}-${item.id}`}\n                        id={`cmd-opt-${globalIndex}`}'
);

fs.writeFileSync('web/src/components/common/CommandPalette.tsx', code);
