#!/bin/bash
# Modal
sed -i 's/import React, { useEffect } from '"'"'react'"'"';/import React, { useEffect } from '"'"'react'"'"';\nimport FocusTrap from '"'"'focus-trap-react'"'"';/' web/src/components/common/Modal.tsx
sed -i 's/return (/return (\n    <FocusTrap active={isOpen}>/' web/src/components/common/Modal.tsx
sed -i 's/    <\/div>\n  );\n};/    <\/div>\n    <\/FocusTrap>\n  );\n};/' web/src/components/common/Modal.tsx

# Drawer
sed -i 's/import React, { useEffect, useState } from '"'"'react'"'"';/import React, { useEffect, useState } from '"'"'react'"'"';\nimport FocusTrap from '"'"'focus-trap-react'"'"';/' web/src/components/common/Drawer.tsx
sed -i 's/return (/return (\n    <FocusTrap active={isOpen}>/' web/src/components/common/Drawer.tsx
sed -i 's/    <\/div>\n  );\n};/    <\/div>\n    <\/FocusTrap>\n  );\n};/' web/src/components/common/Drawer.tsx
