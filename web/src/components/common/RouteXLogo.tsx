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
          className="absolute -inset-1 rounded-lg bg-accent/20 opacity-40 blur-sm transition-opacity duration-200 pointer-events-none"
          aria-hidden="true"
        />
      )}
      <svg
        width={size}
        height={size}
        viewBox="0 0 48 48"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="relative z-10"
        aria-hidden="true"
      >
        {/* Ubin Rute — logo flat Fase 3 (pilihan Arpay): ubin aksen solid
            dengan jalur rute putih; tanpa gradasi, terbaca sampai 16px. */}
        <rect x="2" y="2" width="44" height="44" rx="11" fill="#0070E0" />
        <path
          d="M12 31 H21 L26 17 H36"
          stroke="#FFFFFF"
          strokeWidth="4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle cx="12" cy="31" r="3.4" fill="#FFFFFF" />
        <circle cx="36" cy="17" r="3.4" fill="#FFFFFF" />
      </svg>
    </div>
  );
};
