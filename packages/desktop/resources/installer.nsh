; installer.nsh — Windows integration for the Relay installer.
;
; electron-builder picks this up automatically: it defaults nsis.include to
; <buildResources>/installer.nsh, and buildResources is `resources`.
;
; Three things are added, all under HKCU so no administrator rights are needed
; and nothing is left behind for other users of the machine:
;   1. the bundled `relay` CLI on PATH
;   2. an "Open in Relay" entry when right-clicking a folder
;   3. shortcuts, which electron-builder already creates
;
; Everything here is reversed in customUnInstall.

!include "LogicLib.nsh"
!include "WinMessages.nsh"

; The bundled daemon lives beside the app, in resources\bin. That directory is
; what goes on PATH, so `relay` works in a terminal after installing.
!define RELAY_BIN_DIR "$INSTDIR\resources\bin"
!define ENV_KEY "Environment"
!define MENU_KEY "Software\Classes\Directory\shell\OpenWithRelay"
!define MENU_BG_KEY "Software\Classes\Directory\Background\shell\OpenWithRelay"

; ── PATH ─────────────────────────────────────────────────────────────────────
; Appends our directory to the per-user PATH.
;
; NSIS string buffers are finite, and a PATH longer than the buffer would come
; back truncated. Writing that back would silently destroy entries the user
; depends on, which is far worse than not having `relay` on PATH, so this bails
; out instead of guessing.
!macro RelayAddToPath
  ReadRegStr $0 HKCU "${ENV_KEY}" "Path"
  StrLen $1 $0
  ${If} $1 > 3000
    DetailPrint "Relay: PATH is very long, leaving it untouched to avoid truncating it."
  ${Else}
    ; Skip when it is already there, so repeat installs do not stack copies.
    ; ${StrContains} comes from electron-builder's own NSIS includes.
    ${StrContains} $2 "${RELAY_BIN_DIR}" "$0"
    ${If} $2 == ""
      ${If} $0 == ""
        StrCpy $3 "${RELAY_BIN_DIR}"
      ${Else}
        StrCpy $3 "$0;${RELAY_BIN_DIR}"
      ${EndIf}
      WriteRegExpandStr HKCU "${ENV_KEY}" "Path" "$3"
      ; Tell already-running processes, otherwise a new terminal is needed.
      SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:${ENV_KEY}" /TIMEOUT=2000
      DetailPrint "Relay: added the relay command to your PATH."
    ${EndIf}
  ${EndIf}
!macroend

!macro RelayRemoveFromPath
  ReadRegStr $0 HKCU "${ENV_KEY}" "Path"
  StrLen $1 $0
  ${If} $1 > 3000
    DetailPrint "Relay: PATH is very long, leaving it untouched."
  ${Else}
    Push $0
    Push ";${RELAY_BIN_DIR}"
    Call un.RelayStrRemove
    Pop $3
    Push $3
    Push "${RELAY_BIN_DIR}"
    Call un.RelayStrRemove
    Pop $3
    WriteRegExpandStr HKCU "${ENV_KEY}" "Path" "$3"
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:${ENV_KEY}" /TIMEOUT=2000
  ${EndIf}
!macroend

; ── "Open in Relay" ──────────────────────────────────────────────────────────
; Registered for a folder itself and for the empty space inside a folder, which
; are two different shell verbs. %V is the folder path in both cases.
!macro RelayRegisterContextMenu
  WriteRegStr HKCU "${MENU_KEY}" "" "Open in Relay"
  WriteRegStr HKCU "${MENU_KEY}" "Icon" "$INSTDIR\${APP_EXECUTABLE_FILENAME}"
  WriteRegStr HKCU "${MENU_KEY}\command" "" '"$INSTDIR\${APP_EXECUTABLE_FILENAME}" "%V"'

  WriteRegStr HKCU "${MENU_BG_KEY}" "" "Open in Relay"
  WriteRegStr HKCU "${MENU_BG_KEY}" "Icon" "$INSTDIR\${APP_EXECUTABLE_FILENAME}"
  WriteRegStr HKCU "${MENU_BG_KEY}\command" "" '"$INSTDIR\${APP_EXECUTABLE_FILENAME}" "%V"'
  DetailPrint "Relay: added Open in Relay to the folder right-click menu."
!macroend

!macro RelayUnregisterContextMenu
  DeleteRegKey HKCU "${MENU_KEY}"
  DeleteRegKey HKCU "${MENU_BG_KEY}"
!macroend

; ── Shortcuts ────────────────────────────────────────────────────────────────
; Desktop and Start Menu shortcuts are created by electron-builder itself, see
; createDesktopShortcut and createStartMenuShortcut in electron-builder.yml.
;
; Taskbar pinning is deliberately not attempted. Windows 10 and later block
; programmatic pinning on purpose, and the workarounds that exist rely on
; undocumented shell verbs that break between releases. A user can pin from the
; Start Menu entry in one right-click, which is the supported path.

; ── Small string helpers ─────────────────────────────────────────────────────
; NSIS compiles the installer and the uninstaller as separate passes, and each
; pass warns about any function it does not call. electron-builder promotes that
; warning to an error, so each helper is defined only in the pass that uses it.

!ifdef BUILD_UNINSTALLER
; un.RelayStrRemove: pops the text to remove, then the haystack; pushes the result.
Function un.RelayStrRemove
  Exch $R0 ; needle
  Exch
  Exch $R1 ; haystack
  Push $R2
  Push $R3
  Push $R4
  Push $R5
  StrLen $R2 $R0
  StrCpy $R3 ""
  StrCpy $R4 0
  rloop:
    StrCpy $R5 $R1 $R2 $R4
    StrCmp $R5 "" rdone
    StrCmp $R5 $R0 rskip
    StrCpy $R5 $R1 1 $R4
    StrCpy $R3 "$R3$R5"
    IntOp $R4 $R4 + 1
    Goto rloop
  rskip:
    IntOp $R4 $R4 + $R2
    Goto rloop
  rdone:
    StrCpy $R0 $R3
    Pop $R5
    Pop $R4
    Pop $R3
    Pop $R2
    Exch $R0
    Exch
    Pop $R1
FunctionEnd
!endif

; ── electron-builder hooks ───────────────────────────────────────────────────
!macro customInstall
  !insertmacro RelayAddToPath
  !insertmacro RelayRegisterContextMenu
!macroend

!macro customUnInstall
  !insertmacro RelayUnregisterContextMenu
  !insertmacro RelayRemoveFromPath
!macroend
