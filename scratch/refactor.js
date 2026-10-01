const fs = require('fs');
const path = require('path');

const filesToProcess = [
  'web/src/pages/Dashboard.tsx',
  'web/src/components/dashboard/DashboardHero.tsx',
  'web/src/components/dashboard/QuickStartGuide.tsx',
  'web/src/components/dashboard/PerformanceMetricsRibbon.tsx',
  'web/src/components/dashboard/TrafficChart.tsx',
  'web/src/components/dashboard/LiveRequestsFeed.tsx',
  'web/src/components/dashboard/ProviderHealthMatrix.tsx',
  'web/src/components/dashboard/SystemTelemetrySection.tsx'
];

function processFile(filePath) {
  const fullPath = path.join('/root/Route-X', filePath);
  let content = fs.readFileSync(fullPath, 'utf8');

  // Dashboard.tsx
  // className="border border-border rounded-card overflow-hidden"
  // => className="shadow-md bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl overflow-hidden"
  
  // Replace card wrappers
  content = content.replace(/bg-bg-surface border border-border rounded-card/g, 'bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg');
  
  // Replace border border-border with ring-1 ring-white/5 where bg-bg-surface is not present
  content = content.replace(/border border-border/g, 'ring-1 ring-white/5 border-transparent');
  content = content.replace(/border-t border-border/g, 'border-t border-white/5');
  content = content.replace(/border-r border-border/g, 'border-r border-white/5');
  content = content.replace(/border-b border-border/g, 'border-b border-white/5');
  content = content.replace(/border-l border-border/g, 'border-l border-white/5');

  content = content.replace(/hover:border-border-hover/g, 'hover:ring-white/20');
  content = content.replace(/rounded-card/g, 'rounded-2xl');
  content = content.replace(/rounded-inner/g, 'rounded-xl');

  // Change bg-bg-surface-2 to have backdrop and less opacity
  content = content.replace(/bg-bg-surface-2/g, 'bg-bg-surface-2/60 backdrop-blur-sm');
  
  // Remove shadows that we just replaced to avoid duplicate shadows like shadow-lg shadow-sm
  content = content.replace(/shadow-lg(.*?)shadow-sm/g, 'shadow-lg$1');
  
  fs.writeFileSync(fullPath, content, 'utf8');
  console.log(`Processed ${filePath}`);
}

filesToProcess.forEach(processFile);
