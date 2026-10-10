import React from 'react';

export interface RouteXLogoProps {
  className?: string;
  size?: number;
  glow?: boolean;
}

export const RouteXLogo: React.FC<RouteXLogoProps> = ({
  className = '',
  size = 36,
  glow = true,
}) => {
  return (
    <div className={`relative inline-flex items-center justify-center shrink-0 ${className}`}>
      {glow && (
        <div
          className="absolute -inset-1.5 rounded-2xl bg-gradient-to-r from-blue-600 via-cyan-400 to-indigo-600 opacity-50 blur-md transition-opacity duration-300 group-hover:opacity-85 pointer-events-none"
          aria-hidden="true"
        />
      )}
      <svg
        width={size}
        height={size}
        viewBox="0 0 40 40"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="relative z-10 drop-shadow-md"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="rx-prism-grad-primary" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#38BDF8" />
            <stop offset="50%" stopColor="#3B82F6" />
            <stop offset="100%" stopColor="#6366F1" />
          </linearGradient>

          <linearGradient id="rx-prism-grad-cyan" x1="0%" y1="100%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#06B6D4" />
            <stop offset="60%" stopColor="#38BDF8" />
            <stop offset="100%" stopColor="#60A5FA" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-top" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#38BDF8" stopOpacity="0.45" />
            <stop offset="100%" stopColor="#3B82F6" stopOpacity="0.15" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-left" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#06B6D4" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#0284C7" stopOpacity="0.1" />
          </linearGradient>

          <linearGradient id="rx-prism-facet-right" x1="100%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#6366F1" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#2563EB" stopOpacity="0.1" />
          </linearGradient>

          <linearGradient id="rx-conduit-stroke-1" x1="10%" y1="10%" x2="90%" y2="90%">
            <stop offset="0%" stopColor="#06B6D4" />
            <stop offset="50%" stopColor="#38BDF8" />
            <stop offset="100%" stopColor="#818CF8" />
          </linearGradient>

          <linearGradient id="rx-conduit-stroke-2" x1="90%" y1="10%" x2="10%" y2="90%">
            <stop offset="0%" stopColor="#818CF8" />
            <stop offset="50%" stopColor="#3B82F6" />
            <stop offset="100%" stopColor="#06B6D4" />
          </linearGradient>

          <filter id="rx-core-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur in="SourceGraphic" stdDeviation="1.5" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        {/* Outer Prism Hexagonal Container */}
        <polygon
          points="20,2.5 35.5,11.25 35.5,28.75 20,37.5 4.5,28.75 4.5,11.25"
          fill="#070A10"
          stroke="rgba(255, 255, 255, 0.12)"
          strokeWidth="1.2"
        />

        {/* Facet Geometries */}
        {/* Top Facet */}
        <polygon
          points="20,4.5 33.5,12 20,19.5 6.5,12"
          fill="url(#rx-prism-facet-top)"
          stroke="rgba(56, 189, 248, 0.25)"
          strokeWidth="0.8"
        />

        {/* Left Facet */}
        <polygon
          points="6.5,13.5 18.8,20.5 18.8,35 6.5,27.8"
          fill="url(#rx-prism-facet-left)"
          stroke="rgba(6, 182, 212, 0.2)"
          strokeWidth="0.8"
        />

        {/* Right Facet */}
        <polygon
          points="21.2,20.5 33.5,13.5 33.5,27.8 21.2,35"
          fill="url(#rx-prism-facet-right)"
          stroke="rgba(99, 102, 241, 0.2)"
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
          fill="#0B0F19"
          stroke="url(#rx-prism-grad-cyan)"
          strokeWidth="1.4"
        />

        {/* Edge Routing Nodes */}
        <circle cx="9.5" cy="14" r="2.2" fill="#06B6D4" stroke="#FFFFFF" strokeWidth="0.6" />
        <circle cx="30.5" cy="14" r="2.2" fill="#818CF8" stroke="#FFFFFF" strokeWidth="0.6" />
        <circle cx="9.5" cy="26" r="2.2" fill="#3B82F6" stroke="#FFFFFF" strokeWidth="0.6" />
        <circle cx="30.5" cy="26" r="2.2" fill="#06B6D4" stroke="#FFFFFF" strokeWidth="0.6" />

        {/* Core Nexus Center Glow */}
        <circle
          cx="20"
          cy="20"
          r="2.8"
          fill="#FFFFFF"
          filter="url(#rx-core-glow)"
        />
      </svg>
    </div>
  );
};
