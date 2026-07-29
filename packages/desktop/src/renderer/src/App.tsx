// App.tsx — the shell. Title bar + sidebar + routed content, wrapped in the
// store and toast providers. A connection gate covers the content when the
// daemon can't be reached; first run drops the user into onboarding.

import React, { useState } from 'react'
import { StoreProvider, useStore } from './lib/store'
import { ToastProvider } from './lib/toast'
import { TitleBar } from './components/TitleBar'
import { Sidebar, type Route } from './components/Sidebar'
import { Button, Spinner } from './components/ui'
import { Icon } from './components/Icon'

import { Dashboard } from './screens/Dashboard'
import { Accounts } from './screens/Accounts'
import { Detect } from './screens/Detect'
import { Workflow } from './screens/Workflow'
import { Providers } from './screens/Providers'
import { Pipelines } from './screens/Pipelines'
import { History } from './screens/History'
import { Console } from './screens/Console'
import { Settings } from './screens/Settings'
import { Onboarding } from './screens/Onboarding'

/**
 * A GitHub issue pre-filled with the diagnostics text, so a report carries the
 * one thing that actually helps (what the resolver tried, what the binary
 * said) instead of "it doesn't work." Nothing leaves the machine until the
 * user reviews the pre-filled form and clicks submit themselves; there is no
 * telemetry collector to build or a privacy policy to write for that.
 */
function buildReportUrl(text: string): string {
  const capped = text.length > 3500 ? `${text.slice(0, 3500)}\n... (truncated)` : text
  const body = [
    'What were you doing when this happened?',
    '',
    '',
    'Diagnostics (auto-filled below; review before submitting):',
    '```',
    capped,
    '```',
  ].join('\n')
  const params = new URLSearchParams({ title: 'Daemon failed to connect', labels: 'bug', body })
  return `https://github.com/dbisina/relay/issues/new?${params.toString()}`
}

/** Everything needed to diagnose a failed start, in one copyable block. */
function DaemonDiagnostics() {
  const { connDetail, daemonInfo } = useStore()
  const [copied, setCopied] = useState(false)
  if (!daemonInfo && !connDetail) return null

  const lines: string[] = []
  if (connDetail) lines.push(connDetail, '')
  if (daemonInfo) {
    lines.push(`binary:  ${daemonInfo.binaryPath}${daemonInfo.binaryFound ? '' : '   NOT FOUND'}`)
    if (!daemonInfo.binaryFound && daemonInfo.triedPaths.length > 0) {
      lines.push('looked in:')
      daemonInfo.triedPaths.forEach((p) => lines.push(`  ${p}`))
    }
    if (daemonInfo.probe) lines.push(`probe:   ${daemonInfo.probe}`)
    if (daemonInfo.workDir) lines.push(`workdir: ${daemonInfo.workDir}`)
    if (daemonInfo.logPath) lines.push(`log:     ${daemonInfo.logPath}`)
    if (daemonInfo.logTail.length > 0) {
      lines.push('', 'daemon output:')
      daemonInfo.logTail.forEach((l) => lines.push(`  ${l}`))
    }
  }
  const text = lines.join('\n')

  return (
    <div style={{ width: '100%', maxWidth: 560 }}>
      <div
        className="selectable"
        style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 'var(--fz-xs)',
          color: 'var(--tx-2)',
          background: 'var(--bg-1)',
          border: '1px solid var(--border-0)',
          borderRadius: 'var(--r)',
          padding: '10px 12px',
          textAlign: 'left',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          lineHeight: 1.65,
          maxHeight: 220,
          overflow: 'auto',
        }}
      >
        {text}
      </div>
      <div style={{ display: 'flex', gap: 'var(--s2)', marginTop: 'var(--s2)', justifyContent: 'center' }}>
        <Button
          variant="ghost"
          icon={copied ? 'check' : 'link'}
          onClick={() => {
            navigator.clipboard?.writeText(text)
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          }}
        >
          {copied ? 'Copied' : 'Copy diagnostics'}
        </Button>
        {daemonInfo?.logPath && (
          <Button variant="ghost" icon="folder" onClick={() => window.relay.openLogFolder()}>
            Show log file
          </Button>
        )}
        <Button
          variant="ghost"
          icon="external"
          onClick={() => window.relay.openExternal(buildReportUrl(text))}
        >
          Report on GitHub
        </Button>
      </div>
    </div>
  )
}

function ConnectionGate() {
  const { conn, reconnect } = useStore()
  const [repairing, setRepairing] = useState(false)
  if (conn === 'up') return null

  const repair = async () => {
    setRepairing(true)
    await window.relay.repairDaemon()
    setRepairing(false)
  }
  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        zIndex: 100,
        background: 'var(--bg-0)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 'var(--s4)',
        textAlign: 'center',
        padding: 'var(--s6)',
      }}
    >
      {conn === 'failed' ? (
        <>
          <div
            style={{
              width: 52,
              height: 52,
              borderRadius: 'var(--r-lg)',
              border: '1px solid var(--border-1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--red)',
            }}
          >
            <Icon name="warning" size={24} />
          </div>
          <div>
            <div style={{ fontSize: 'var(--fz-xl)', fontWeight: 600 }}>Daemon not reachable</div>
            <div
              style={{
                fontSize: 'var(--fz-md)',
                color: 'var(--tx-2)',
                maxWidth: 460,
                lineHeight: 1.55,
                marginTop: 8,
              }}
            >
              The daemon ships inside this app and is started automatically, so this means it could
              not be launched. The details below say why.
            </div>
          </div>
          <DaemonDiagnostics />
          <div style={{ display: 'flex', gap: 'var(--s2)' }}>
            <Button variant="primary" icon="refresh" loading={repairing} onClick={repair}>
              {repairing ? 'Repairing…' : 'Repair and retry'}
            </Button>
            <Button variant="ghost" onClick={reconnect}>
              Just retry
            </Button>
            <Button
              variant="ghost"
              icon="external"
              onClick={() => window.relay.openExternal('https://dbisina.github.io/relay')}
            >
              Install guide
            </Button>
          </div>
        </>
      ) : (
        <>
          <Spinner size={26} />
          <div style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>
            {conn === 'starting' ? 'Starting the Relay daemon' : 'Connecting to Relay'}
          </div>
          <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
            The daemon holds all state; this window is just the cockpit.
          </div>
        </>
      )}
    </div>
  )
}

const SCREENS: Record<Route, React.ComponentType<{ go: (r: Route) => void }>> = {
  dashboard: Dashboard,
  accounts: Accounts,
  detect: Detect,
  workflow: Workflow,
  providers: Providers,
  pipelines: Pipelines,
  history: History,
  console: Console,
  settings: Settings,
}

// Home manages its own internal scroll (fixed composer at the bottom, a
// scrollable session/event area above it) — the shared <main> wrapper must not
// also scroll it, or the composer stops being pinned. Every other screen uses
// the <Page> scaffold, which relies on <main> itself scrolling.
const SELF_SCROLLING: Partial<Record<Route, true>> = { dashboard: true }

function Shell() {
  const [route, setRoute] = useState<Route>('dashboard')
  const [onboarded, setOnboarded] = useState(() => localStorage.getItem('relay.onboarded') === '1')
  const { conn } = useStore()

  // Whether setup is done is decided by this flag alone, never inferred from
  // daemon data. Inferring it re-runs whenever providers load, which is exactly
  // when someone is partway through the wizard, and it yanked them out mid-step.
  // A returning user with no flag just sees the wizard and clicks Skip once.

  const finishOnboarding = () => {
    localStorage.setItem('relay.onboarded', '1')
    setOnboarded(true)
    setRoute('dashboard')
  }

  const Screen = SCREENS[route]
  // Onboarding does NOT wait for the daemon. Picking a language and reading
  // what the tools are is useful while it boots, and the steps that do need it
  // disable just themselves. Only a hard failure takes the screen.
  const showOnboarding = !onboarded

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <TitleBar />
      <div style={{ flex: 1, display: 'flex', minHeight: 0, position: 'relative' }}>
        {showOnboarding ? (
          <Onboarding onDone={finishOnboarding} />
        ) : (
          <>
            <Sidebar route={route} onNavigate={setRoute} />
            <main style={{ flex: 1, minWidth: 0, overflow: SELF_SCROLLING[route] ? 'hidden' : 'auto', position: 'relative' }}>
              <Screen go={setRoute} />
            </main>
          </>
        )}
        {/* Only a real failure covers the app. A daemon still starting is
            reported inline by whatever needs it. */}
        {(!showOnboarding || conn === 'failed') && <ConnectionGate />}
      </div>
    </div>
  )
}

export function App() {
  return (
    <ToastProvider>
      <StoreProvider>
        <Shell />
      </StoreProvider>
    </ToastProvider>
  )
}
