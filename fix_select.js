const fs = require('fs');
let code = fs.readFileSync('web/src/components/common/Select.tsx', 'utf8');

// Add id to options and activedescendant
code = code.replace(
    '<input',
    '<input\n                aria-activedescendant={isOpen && highlightedIndex >= 0 ? `${selectId}-opt-${highlightedIndex}` : undefined}'
);
code = code.replace(
    '<div\n            ref={listRef}',
    '<div\n            id={`${selectId}-listbox`}\n            ref={listRef}'
);
code = code.replace(
    'aria-haspopup="listbox"',
    'aria-haspopup="listbox"\n        aria-controls={isOpen ? `${selectId}-listbox` : undefined}'
);
code = code.replace(
    'key={opt.value || `empty-${index}`}',
    'key={opt.value || `empty-${index}`}\n                    id={`${selectId}-opt-${index}`}'
);
fs.writeFileSync('web/src/components/common/Select.tsx', code);
