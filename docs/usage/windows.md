# windows

anfra ships for Linux and macOS only. On Windows, run it inside WSL2 (Windows Subsystem for Linux), which gives you a real Linux system next to Windows. Your browser, editor and coding agent on Windows can still reach it.

## 1. Install WSL2

You need Windows 10 version 2004 or later, or Windows 11. Open PowerShell **as Administrator** and run:

```powershell
wsl --install
```

This turns on WSL, installs the latest WSL2 kernel and adds Ubuntu. Restart when it asks. When Ubuntu first starts, it asks for a Linux username and password; they are separate from your Windows account.

If WSL was already installed, make sure it runs version 2:

```powershell
wsl --update
wsl --set-default-version 2
wsl -l -v          # the VERSION column should say 2
```

## 2. Install anfra inside WSL

Open Ubuntu (from the Start menu, or `wsl` in a terminal) and run:

```sh
curl -fsSL https://anfra.ai/install.sh | bash
```

Open a new Ubuntu terminal so `PATH` picks up `~/.anfra/bin`, then check it:

```sh
anfra version
```

Run every `anfra` command from inside WSL. The Linux binary does not run from PowerShell or `cmd`.

## 3. Keep your repo in the Linux file system

Create or clone your repo under your Linux home, for example `~/projects/my-data`, not under `/mnt/c/...`. Files on the Windows drive are much slower to read from WSL, and file watching (which `anfra serve` uses to reload) is unreliable there.

To open the folder in Windows tools:

- **VS Code**: install the WSL extension, then run `code .` inside the repo in Ubuntu.
- **File Explorer**: run `explorer.exe .`, or go to `\\wsl$\Ubuntu\home\<you>`.

## 4. Open `anfra serve` from Windows

Run `anfra serve` in WSL as usual. WSL2 forwards `localhost`, so the link it prints opens in your Windows browser as is.

If the page doesn't load, check that localhost forwarding hasn't been turned off: `%UserProfile%\.wslconfig` must not set `localhostForwarding=false`. After changing that file, run `wsl --shutdown` and start Ubuntu again.

## 5. Coding agents

Run your coding agent (Claude Code, Codex, ...) inside WSL too, in the repo's folder, so it can call `anfra` and read the repo. `anfra skills install` then installs the skills for the agent in WSL.

## Troubleshooting

| Problem | Fix |
|---|---|
| `wsl --install` prints help instead of installing | WSL is already on. Run `wsl --install -d Ubuntu` to add the distribution. |
| "Please enable the Virtual Machine Platform" or a virtualization error | Turn on virtualization (Intel VT-x or AMD-V) in your BIOS/UEFI settings. |
| `anfra: command not found` | Open a new terminal, or call it by its full path, `~/.anfra/bin/anfra`. |
| A database on Windows (`localhost:5432`) can't be reached from WSL | In WSL, Windows' `localhost` is not WSL's. Use the Windows host's IP (`ip route show default \| awk '{print $3}'`), or turn on mirrored networking: `networkingMode=mirrored` under `[wsl2]` in `.wslconfig` (Windows 11 22H2 or later). |
