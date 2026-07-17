; scripts/installer.iss — Inno Setup script for Relay Windows installer.
; Produces dist\relay-setup.exe.
; Build:  iscc /DMyAppVersion=v0.1.0 scripts\installer.iss
; CI:     called automatically by release.yml for the windows-amd64 matrix.

#ifndef MyAppVersion
  #define MyAppVersion "dev"
#endif

#define MyAppName      "Relay"
#define MyAppPublisher "Daniel Bisina"
#define MyAppURL       "https://github.com/dbisina/relay"
#define MyAppExeName   "relay-ui.exe"

[Setup]
AppId={{F3A2B7C1-4D8E-4F9A-B3C2-1E5D7A9F2B4C}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppUpdatesURL={#MyAppURL}/releases
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile=..\LICENSE
OutputDir=..\dist
OutputBaseFilename=relay-setup
SetupIconFile=..\packages\ui\assets\relay.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
; Require admin to write to Program Files and set system PATH
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
; Minimum Windows 10
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "addtopath";   Description: "Add Relay CLI to PATH (recommended)"; GroupDescription: "Shell integration:"; Flags: checkedonce
Name: "startdaemon"; Description: "Start the Relay daemon on login (keeps the CLI working while the app is closed)"; GroupDescription: "Background service:"; Flags: checkedonce

[Registry]
; Launch the daemon (headless, no window) at login so `relay` CLI and the app
; always share one running orchestrator. Removed cleanly on uninstall.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "RelayDaemon"; ValueData: """{app}\relay.exe"" daemon"; Flags: uninsdeletevalue; Tasks: startdaemon

[Files]
Source: "..\dist\relay.exe";    DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\relay-ui.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Start Menu
Name: "{group}\{#MyAppName}";            Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{#MyAppName} CLI (docs)"; Filename: "https://github.com/dbisina/relay#readme"
Name: "{group}\Uninstall {#MyAppName}";  Filename: "{uninstallexe}"
; Desktop (optional, unchecked by default)
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[Code]
const
  EnvironmentKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

procedure AddToPath(InstallPath: string);
var
  ExistingPath: string;
begin
  if not IsTaskSelected('addtopath') then Exit;
  RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', ExistingPath);
  if Pos(LowerCase(InstallPath), LowerCase(ExistingPath)) = 0 then
  begin
    RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path',
      ExistingPath + ';' + InstallPath);
    // Broadcast WM_SETTINGCHANGE so open terminals pick up the new PATH
    SendBroadcastMessage($001A {WM_SETTINGCHANGE}, 0, 'Environment');
  end;
end;

procedure RemoveFromPath(InstallPath: string);
var
  ExistingPath, NewPath: string;
  P: Integer;
begin
  RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', ExistingPath);
  P := Pos(';' + LowerCase(InstallPath), LowerCase(ExistingPath));
  if P > 0 then
  begin
    Delete(ExistingPath, P, Length(';' + InstallPath));
    RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', ExistingPath);
    SendBroadcastMessage($001A, 0, 'Environment');
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    AddToPath(ExpandConstant('{app}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;
