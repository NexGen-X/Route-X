const fs = require('fs');

function patch(file, wrapperStart, wrapperEnd) {
    let code = fs.readFileSync(file, 'utf8');
    if (!code.includes('FocusTrap')) {
        code = code.replace("import React, { useEffect } from 'react';", "import React, { useEffect } from 'react';\nimport FocusTrap from 'focus-trap-react';");
        code = code.replace("import React, { useState, useEffect, useRef } from 'react';", "import React, { useState, useEffect, useRef } from 'react';\nimport FocusTrap from 'focus-trap-react';");
        code = code.replace("import React, { useState, useEffect } from 'react';", "import React, { useState, useEffect } from 'react';\nimport FocusTrap from 'focus-trap-react';");
        
        // CommandPalette doesn't have open state passed to FocusTrap, it just renders when open
        if (file.includes('CommandPalette')) {
           code = code.replace(
                '<div className="fixed inset-0 z-50',
                '<FocusTrap focusTrapOptions={{ initialFocus: false, clickOutsideDeactivates: true }}><div className="fixed inset-0 z-50"'
           );
           code = code.replace(
                '    </div>\n  );\n};\n',
                '    </div></FocusTrap>\n  );\n};\n'
           );
        } else {
           code = code.replace(
                '<div className={`fixed inset-0 z-50 flex',
                wrapperStart + '<div className={`fixed inset-0 z-50 flex'
           );
           
           code = code.replace(
                '    </div>\n  );\n};\n',
                '    </div>\n' + wrapperEnd + '  );\n};\n'
           );
        }
        
        fs.writeFileSync(file, code);
    }
}

patch('web/src/components/common/Modal.tsx', '<FocusTrap active={isOpen}>', '</FocusTrap>');
patch('web/src/components/common/Drawer.tsx', '<FocusTrap active={isOpen}>', '</FocusTrap>');
patch('web/src/components/common/CommandPalette.tsx', '<FocusTrap focusTrapOptions={{ initialFocus: false, clickOutsideDeactivates: true }}>', '</FocusTrap>');
