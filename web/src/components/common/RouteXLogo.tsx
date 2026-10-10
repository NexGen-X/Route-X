import React from 'react';

export interface RouteXLogoProps {
  className?: string;
  size?: number;
  glow?: boolean;
}

export const RouteXLogo: React.FC<RouteXLogoProps> = ({
  className = '',
  size = 36,
  glow = false,
}) => {
  return (
    <div className={`relative inline-flex items-center justify-center shrink-0 ${className}`}>
      {glow && (
        <div
          className="absolute -inset-1 rounded-lg bg-indigo-500/20 opacity-40 blur-sm transition-opacity duration-200 pointer-events-none"
          aria-hidden="true"
        />
      )}
      <svg
        width={size}
        height={size}
        viewBox="0 0 40 40"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="relative z-10"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="rx-prism-grad-primary" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#6366F1" />
            <stop offset="100%" stopColor="#3B82F6" />
          </linearGradient>

          <linearGradient id="rx-prism-grad-cyan" x1="0%" y1="100%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#06B6D4" />
            <stop offset="100%" stopColor="#6366F1" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-top" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#6366F1" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#3B82F6" stopOpacity="0.10" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-left" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#06B6D4" stopOpacity="0.30" />
            <stop offset="100%" stopColor="#6366F1" stopOpacity="0.08" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-right" x1="100%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#3B82F6" stopOpacity="0.30" />
            <stop offset="100%" stopColor="#6366F1" stopOpacity="0.08" />
          </linearGradient>

          <linearGradient id="rx-conduit-stroke-1" x1="10%" y1="10%" x2="90%" y2="90%">
            <stop offset="0%" stopColor="#06B6D4" />
            <stop offset="100%" stopColor="#6366F1" />
          </linearGradient>

          <linearGradient id="rx-conduit-stroke-2" x1="90%" y1="10%" x2="10%" y2="90%">
            <stop offset="0%" stopColor="#6366F1" />
            <stop offset="100%" stopColor="#3B82F6" />
          </linearGradient>
        </defs>

        {/* Outer Prism Hexagonal Container */}
        <polygon
          points="20,2.5 35.5,11.25 35.5,28.75 20,37.5 4.5,28.75 4.5,11.25"
          fill="#121215"
          stroke="rgba(99, 102, 241, 0.3)"
          strokeWidth="1.2"
        />

        {/* Facet Geometries */}
        {/* Top Facet */}
        <polygon
          points="20,4.5 33.5,12 20,19.5 6.5,12"
          fill="url(#rx-prism-facet-top)"
          stroke="rgba(99, 102, 241, 0.35)"
          strokeWidth="0.8"
        />

        {/* Left Facet */}
        <polygon
          points="6.5,13.5 18.8,20.5 18.8,35 6.5,27.8"
          fill="url(#rx-prism-facet-left)"
          stroke="rgba(6, 182, 212, 0.3)"
          strokeWidth="0.8"
        />

        {/* Right Facet */}
        <polygon
          points="21.2,20.5 33.5,13.5 33.5,27.8 21.2,35"
          fill="url(#rx-prism-facet-right)"
          stroke="rgba(59, 130, 246, 0.3)"
          strokeWidth="0.8"
        />

        {/* Interconnected Routing Conduits (Crossing 'X' Vector Flow) */}
        <path
          d="M9.5 14 L20 20 L30.5 26"
          stroke="url(#rx-conduit-stroke-1)"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M30.5 14 L20 20 L9.5 26"
          stroke="url(#rx-conduit-stroke-2)"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {/* Central Router Nexus Crystal */}
        <polygon
          points="20,15.5 24,17.8 24,22.2 20,24.5 16,22.2 16,17.8"
          fill="#18181B"
          stroke="url(#rx-prism-grad-cyan)"
          strokeWidth="1.4"
        />

        {/* Edge Routing Nodes */}
        <circle cx="9.5" cy="14" r="2.2" fill="#06B6D4" stroke="#09090B" strokeWidth="0.8" />
        <circle cx="30.5" cy="14" r="2.2" fill="#6366F1" stroke="#09090B" strokeWidth="0.8" />
        <circle cx="9.5" cy="26" r="2.2" fill="#6366F1" stroke="#09090B" strokeWidth="0.8" />
        <circle cx="30.5" cy="26" r="2.2" fill="#3B82F6" stroke="#09090B" strokeWidth="0.8" />

        {/* Core Nexus Center Dot */}
        <circle
          cx="20"
          cy="20"
          r="2.5"
          fill="#FAFAFA"
        />
      </svg>
    </div>
  );
};
