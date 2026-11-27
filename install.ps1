# anfra installer for Windows.
#
#   irm https://anfra.ai/install.ps1 | iex
#
# Downloads the anfra release for Windows from GitHub Releases, checks it
# against the release's SHA256SUMS, and installs anfra.exe to
# %USERPROFILE%\.anfra\bin (override with ANFRA_HOME or ANFRA_INSTALL_DIR). The
# release embeds both sidecars, so it's large (~130 MB to download).
#
# Environment:
#   ANFRA_HOME           anfra's folder, where it keeps everything (default: %USERPROFILE%\.anfra)
#   ANFRA_INSTALL_DIR    install location (default: $ANFRA_HOME\bin)
#   ANFRA_VERSION        pin a version, e.g. 0.24.0 (default: latest)
#   ANFRA_NO_MODIFY_PATH if set, don't change the user PATH; just print how to
#   ANFRA_DOWNLOAD_URL   the folder to download anfra-windows-x64.zip and
#                        SHA256SUMS from, instead of the GitHub release (for testing)
#
# Run through `iex`, this runs in the user's own PowerShell session: it never
# calls `exit`, which would close their window, and it keeps to ASCII, which
# Windows PowerShell 5.1 and PowerShell 7 read alike.

& {
    $ErrorActionPreference = 'Stop'
    # Windows PowerShell 5.1 draws its progress bar so slowly that it slows
    # the download itself.
    $ProgressPreference = 'SilentlyContinue'
    # Windows PowerShell 5.1 may not offer TLS 1.2, which GitHub requires.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'holistics/anfra'
    $asset = 'anfra-windows-x64.zip'
    $anfraHome = if ($env:ANFRA_HOME) { $env:ANFRA_HOME } else { Join-Path $env:USERPROFILE '.anfra' }
    $installDir = if ($env:ANFRA_INSTALL_DIR) { $env:ANFRA_INSTALL_DIR } else { Join-Path $anfraHome 'bin' }
    $target = Join-Path $installDir 'anfra.exe'

    # --- output helpers (color unless NO_COLOR) ---
    $color = -not $env:NO_COLOR
    function Say([string] $text, [string] $fg) {
        if ($color -and $fg) { Write-Host $text -ForegroundColor $fg } else { Write-Host $text }
    }
    function Ok([string] $text) { Say "OK  $text" 'Green' }
    function Warn([string] $text) { Say "warning: $text" 'Yellow' }
    function Fail([string] $text) { Say "error: $text" 'Red' }
    # Show paths under the user's folder as ~\..., which is shorter.
    function Tilde([string] $path) {
        if ($path.StartsWith($env:USERPROFILE, [StringComparison]::OrdinalIgnoreCase)) { return '~' + $path.Substring($env:USERPROFILE.Length) }
        return $path
    }

    # --- detect the CPU ---
    # 32-bit PowerShell on 64-bit Windows reports the machine's CPU in PROCESSOR_ARCHITEW6432.
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($cpu) {
        'AMD64' { $cpuLabel = 'x64' }
        'ARM64' { $cpuLabel = 'arm64' }
        default {
            Fail "anfra runs on 64-bit Windows (x64 or ARM64), and this computer's CPU is $cpu."
            return
        }
    }

    # --- where to download from ---
    # /releases/latest/download/<file> and /releases/download/<tag>/<file> redirect
    # straight to GitHub's CDN, so there's no API call and no rate limit.
    if ($env:ANFRA_DOWNLOAD_URL) {
        $base = $env:ANFRA_DOWNLOAD_URL.TrimEnd('/')
        $wanted = "anfra from $base"
    } elseif ($env:ANFRA_VERSION) {
        $version = $env:ANFRA_VERSION -replace '^v', ''
        $base = "https://github.com/$repo/releases/download/v$version"
        $wanted = "anfra $version"
    } else {
        $base = "https://github.com/$repo/releases/latest/download"
        $wanted = 'the latest anfra'
    }

    Write-Host ''
    Write-Host "Installing $wanted for Windows $cpuLabel"
    if ($cpuLabel -eq 'arm64') {
        Warn 'anfra has no ARM64 build yet, so this installs the x64 one. Windows runs it through emulation, which is slower.'
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("anfra-install-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        # --- download, and check it against the release's checksums ---
        $zip = Join-Path $tmp $asset
        $sums = Join-Path $tmp 'SHA256SUMS'
        try {
            Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile $sums
            Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $zip
        } catch {
            Fail "Couldn't download $asset or its SHA256SUMS from $base. Check your connection, and that the version exists: $($_.Exception.Message)"
            return
        }

        $expected = $null
        foreach ($line in Get-Content $sums) {
            $fields = -split $line
            if ($fields.Count -eq 2 -and $fields[1].TrimStart('*') -eq $asset) { $expected = $fields[0].ToLower() }
        }
        if (-not $expected) {
            Fail "Not installed, because the release's SHA256SUMS has no line for $asset, so the download can't be checked."
            return
        }
        $actual = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
        if ($actual -ne $expected) {
            Fail "Not installed, because the downloaded $asset doesn't match the checksum its release gives it. It may have been cut short or changed on the way. Try again: irm https://anfra.ai/install.ps1 | iex"
            return
        }

        $unpacked = Join-Path $tmp 'unpacked'
        Expand-Archive -Path $zip -DestinationPath $unpacked
        $exe = Join-Path $unpacked 'anfra.exe'
        if (-not (Test-Path $exe)) {
            Fail "Not installed, because $asset has no anfra.exe at its top."
            return
        }

        # --- install ---
        New-Item -ItemType Directory -Force -Path $installDir | Out-Null
        try {
            Copy-Item -Force $exe $target
        } catch {
            Fail "Couldn't replace $(Tilde $target), because Windows won't overwrite a program that's running. Stop every anfra (anfra serve, anfra console), then run irm https://anfra.ai/install.ps1 | iex again."
            return
        }
        $notice = Join-Path $unpacked 'THIRD_PARTY_NOTICES.txt'
        if (Test-Path $notice) { Copy-Item -Force $notice $installDir }

        # `anfra --version` prints "anfra version X"; keep just the version.
        $installed = (& $target --version 2>$null) -replace '^anfra version ', ''
        Write-Host ''
        Ok "Installed anfra $installed to $(Tilde $target)"
        Say "    sha256 $actual" 'DarkGray'
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    # --- put the install folder on PATH ---
    # Read and write the user PATH as the registry stores it: unexpanded, and
    # keeping its type. [Environment]::SetEnvironmentVariable would store it
    # expanded, turning the user's %USERPROFILE%\... entries into fixed paths.
    $envKey = Get-Item 'HKCU:\Environment'
    $userPath = $envKey.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
    $onUserPath = @($userPath -split ';' | Where-Object {
        [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ieq $installDir.TrimEnd('\')
    }).Count -gt 0
    $restart = $false
    if (-not $onUserPath) {
        if ($env:ANFRA_NO_MODIFY_PATH) {
            Write-Host ''
            Write-Host "$(Tilde $installDir) is not on your PATH. To use anfra from any terminal, add it to your user PATH: Windows Settings > System > About > Advanced system settings > Environment Variables."
        } else {
            $newPath = if ($userPath) { "$installDir;$userPath" } else { $installDir }
            $kind = if ($envKey.GetValueNames() -contains 'Path') { $envKey.GetValueKind('Path') } else { 'ExpandString' }
            Set-ItemProperty -Path 'HKCU:\Environment' -Name 'Path' -Value $newPath -Type $kind
            # Writing the registry doesn't tell open programs, such as Explorer, that the
            # environment changed; setting a user variable does, so new terminals see the PATH.
            [Environment]::SetEnvironmentVariable('ANFRA_INSTALLER', '1', 'User')
            [Environment]::SetEnvironmentVariable('ANFRA_INSTALLER', $null, 'User')
            Ok "Added $(Tilde $installDir) to your user PATH"
            $restart = $true
        }
    }
    # This window is the user's own session, so anfra can work in it right away.
    if (-not (@($env:Path -split ';') -contains $installDir)) { $env:Path = "$installDir;$env:Path" }
    $found = (Get-Command anfra -ErrorAction SilentlyContinue | Select-Object -First 1).Source
    if ($found -and ($found -ine $target)) {
        Warn "another anfra at $(Tilde $found) comes before $(Tilde $target) on your PATH"
    }

    # --- what to do next ---
    Write-Host ''
    if ($restart) {
        Write-Host 'anfra works in this window now. Terminals that were already open need restarting to find it.'
        Write-Host ''
    }
    Say 'Next steps' 'White'
    Write-Host ''
    Say '  1  Install the agent skills' 'White'
    Say '     Your agents need them to set up projects,' 'DarkGray'
    Say '     model data and build apps with Anfra.' 'DarkGray'
    Write-Host ''
    Write-Host '       > anfra agent setup'
    Write-Host ''
    Say '  2  Create a project' 'White'
    Write-Host ''
    Write-Host '       > anfra init my-project'
    Write-Host '       > cd my-project'
    Write-Host ''
    Say '  3  Connect your warehouse' 'White'
    Write-Host '     Fill in your credentials in .anfra\data_sources.yml'
    Say '     Supported warehouses: https://docs.anfra.ai/docs/self-hosted/data-sources' 'DarkGray'
    Write-Host ''
    Say '  4  Build an app' 'White'
    Write-Host '     Open the project in your coding agent and ask:'
    Write-Host ''
    Write-Host '       "Use the anfra-build-data-app skill. Look at my orders data and build'
    Write-Host '       a revenue overview: monthly trend and revenue by region.'
    Write-Host '       Clicking a region should filter the trend."'
    Write-Host ''
    Say '  5  See it running' 'White'
    Write-Host ''
    Write-Host '       > anfra serve'
    Write-Host ''
    Write-Host '     Then open http://127.0.0.1:7878/'
    Say '     Apps reload as you or the agent edit them.' 'DarkGray'
    Write-Host ''
    Say '--------------------------------------------------------------' 'DarkGray'
    Say '  >> Shortcut' 'White'
    Write-Host '     Your coding agent can do steps 2 and 3 for you. Open an'
    Write-Host '     empty folder in it and ask:'
    Write-Host ''
    Write-Host '       "Use the anfra-setup-repo skill to set up Anfra and connect my warehouse."'
    Write-Host ''
    Say '--------------------------------------------------------------' 'DarkGray'
    Write-Host 'Docs      https://docs.anfra.ai'
    Write-Host 'Feedback  https://github.com/holistics/anfra/discussions'
}
