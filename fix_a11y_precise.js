const fs = require('fs');

function wrapWithFocusTrap(file, isCommandPalette) {
    let code = fs.readFileSync(file, 'utf8');
    
    // add import
    code = code.replace("import React,", "import FocusTrap from 'focus-trap-react';\nimport React,");
    if (!code.includes("import FocusTrap")) {
        code = code.replace("import React", "import FocusTrap from 'focus-trap-react';\nimport React");
    }

    if (isCommandPalette) {
        code = code.replace(
            '<div className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] sm:pt-[15vh]">',
            '<FocusTrap focusTrapOptions={{ initialFocus: false, clickOutsideDeactivates: true }}>\n      <div className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] sm:pt-[15vh]" role="dialog" aria-modal="true" aria-label="Command Palette">'
        );
        code = code.replace(
            '    </div>\n  );\n};',
            '      </div>\n    </FocusTrap>\n  );\n};'
        );
        
        // Aria activedescendant
        code = code.replace(
            'placeholder="Ketik perintah atau cari..."',
            'placeholder="Ketik perintah atau cari..."\n            aria-label="Cari perintah"\n            aria-activedescendant={results.length > 0 ? `cmd-opt-${selectedIndex}` : undefined}'
        );
        
        // Option ID
        code = code.replace(
            'key={`${group.id}-${item.id}`}',
            'key={`${group.id}-${item.id}`}\n                        id={`cmd-opt-${globalIndex}`}'
        );
    } else {
        code = code.replace(
            '<div className={`fixed inset-0 z-50 flex',
            '<FocusTrap active={isOpen}>\n      <div className={`fixed inset-0 z-50 flex'
        );
        code = code.replace(
            '    </div>\n  );\n};',
            '      </div>\n    </FocusTrap>\n  );\n};'
        );
    }

    fs.writeFileSync(file, code);
}

wrapWithFocusTrap('web/src/components/common/Modal.tsx', false);
wrapWithFocusTrap('web/src/components/common/Drawer.tsx', false);
wrapWithFocusTrap('web/src/components/common/CommandPalette.tsx', true);
