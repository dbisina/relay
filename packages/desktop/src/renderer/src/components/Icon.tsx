// Icon.tsx — one small, consistent stroke-icon set. currentColor, 1.6 stroke.
// Keeps the whole app on a single visual language without an icon-font dep.

import React from 'react'

export type IconName =
  | 'dashboard'
  | 'accounts'
  | 'detect'
  | 'workflow'
  | 'providers'
  | 'pipelines'
  | 'history'
  | 'settings'
  | 'plus'
  | 'check'
  | 'x'
  | 'chevronRight'
  | 'chevronDown'
  | 'external'
  | 'refresh'
  | 'handoff'
  | 'key'
  | 'download'
  | 'search'
  | 'folder'
  | 'warning'
  | 'skill'
  | 'play'
  | 'pause'
  | 'minus'
  | 'maximize'
  | 'link'

const P: Record<IconName, React.ReactNode> = {
  dashboard: (
    <>
      <rect x="3" y="3" width="7" height="9" rx="1.4" />
      <rect x="14" y="3" width="7" height="5" rx="1.4" />
      <rect x="14" y="12" width="7" height="9" rx="1.4" />
      <rect x="3" y="16" width="7" height="5" rx="1.4" />
    </>
  ),
  accounts: (
    <>
      <circle cx="9" cy="8" r="3.2" />
      <path d="M3.5 20c0-3.2 2.6-5 5.5-5s5.5 1.8 5.5 5" />
      <path d="M16 5.2a3 3 0 0 1 0 5.6M18 20c0-2.4-1-4-2.6-4.7" />
    </>
  ),
  detect: (
    <>
      <circle cx="12" cy="12" r="2.4" />
      <path d="M12 3.5a8.5 8.5 0 0 1 8.5 8.5" />
      <path d="M12 7.5a4.5 4.5 0 0 1 4.5 4.5" />
      <path d="M4 20l4.2-4.2" />
    </>
  ),
  workflow: (
    <>
      <circle cx="6" cy="6" r="2.4" />
      <circle cx="18" cy="6" r="2.4" />
      <circle cx="12" cy="18" r="2.4" />
      <path d="M6 8.4v2.1a2 2 0 0 0 2 2h3M18 8.4v2.1a2 2 0 0 1-2 2h-3M12 13v2.6" />
    </>
  ),
  providers: (
    <>
      <path d="M9 3v5M15 3v5" />
      <rect x="6" y="8" width="12" height="6" rx="2" />
      <path d="M12 14v4a3 3 0 0 1-3 3" />
    </>
  ),
  pipelines: (
    <>
      <circle cx="6" cy="6" r="2.2" />
      <circle cx="6" cy="18" r="2.2" />
      <circle cx="18" cy="12" r="2.2" />
      <path d="M8.2 6H13a3 3 0 0 1 3 3v.8M8.2 18H13a3 3 0 0 0 3-3v-.8" />
    </>
  ),
  history: (
    <>
      <path d="M3.5 12a8.5 8.5 0 1 0 2.6-6.1" />
      <path d="M5.5 3v3h3" />
      <path d="M12 7.5V12l3 1.8" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2.5v2.2M12 19.3v2.2M4.6 4.6l1.6 1.6M17.8 17.8l1.6 1.6M2.5 12h2.2M19.3 12h2.2M4.6 19.4l1.6-1.6M17.8 6.2l1.6-1.6" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  minus: <path d="M5 12h14" />,
  check: <path d="M4 12.5l5 5 11-11" />,
  x: <path d="M6 6l12 12M18 6L6 18" />,
  chevronRight: <path d="M9 5l7 7-7 7" />,
  chevronDown: <path d="M5 9l7 7 7-7" />,
  external: (
    <>
      <path d="M14 4h6v6" />
      <path d="M20 4l-9 9" />
      <path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 11a8 8 0 0 0-14-4.5L4 8" />
      <path d="M4 4v4h4" />
      <path d="M4 13a8 8 0 0 0 14 4.5L20 16" />
      <path d="M20 20v-4h-4" />
    </>
  ),
  handoff: (
    <>
      <path d="M3 8h13" />
      <path d="M12 4l4 4-4 4" />
      <path d="M21 16H8" />
      <path d="M12 12l-4 4 4 4" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="8" r="4" />
      <path d="M11 11l9 9M17 17l2-2M15 15l2-2" />
    </>
  ),
  download: (
    <>
      <path d="M12 3v12" />
      <path d="M7 11l5 5 5-5" />
      <path d="M4 20h16" />
    </>
  ),
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6" />
      <path d="M20 20l-5-5" />
    </>
  ),
  folder: <path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2.5h8.5A1.5 1.5 0 0 1 21 9v8.5a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z" />,
  warning: (
    <>
      <path d="M12 3.5l9.5 16.5H2.5z" />
      <path d="M12 9.5v5M12 17.5v.01" />
    </>
  ),
  skill: (
    <>
      <path d="M12 3l2.3 5.3L20 9l-4 3.8L17 19l-5-3-5 3 1-6.2L4 9l5.7-.7z" />
    </>
  ),
  play: <path d="M7 4.5v15l13-7.5z" />,
  pause: (
    <>
      <rect x="6" y="4.5" width="4" height="15" rx="1" />
      <rect x="14" y="4.5" width="4" height="15" rx="1" />
    </>
  ),
  maximize: <rect x="4.5" y="4.5" width="15" height="15" rx="2" />,
  link: (
    <>
      <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7L11.5 7" />
      <path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7L12.5 17" />
    </>
  ),
}

export function Icon({
  name,
  size = 16,
  strokeWidth = 1.6,
  style,
  className,
}: {
  name: IconName
  size?: number
  strokeWidth?: number
  style?: React.CSSProperties
  className?: string
}) {
  const filled = name === 'play' || name === 'skill' || name === 'pause' || name === 'maximize'
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke={filled ? 'none' : 'currentColor'}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ flexShrink: 0, ...style }}
      className={className}
      aria-hidden="true"
    >
      {P[name]}
    </svg>
  )
}
