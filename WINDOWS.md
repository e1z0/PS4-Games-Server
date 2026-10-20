# Windows: start with the EXE

You need a Windows PC, your PS4 on the same home network, and your `.pkg` files. You do not need Go, Docker, Python or Node.js. Keep the PC awake while the PS4 downloads.

## 1. Download and unpack

Download the Windows **amd64** ZIP for most Intel/AMD PCs. Choose **arm64** only for Windows on ARM. Extract the ZIP with **Extract All**; do not run the EXE inside the ZIP viewer.

Use this example layout (replace the package drive if needed):

```text
C:\PS4Server\ps4-server.exe
D:\PS4Packages\Games\Example Game.pkg
D:\PS4Packages\Updates\Example Update.pkg
D:\PS4Packages\DLC\Example DLC.pkg
```

Only the EXE is needed to run the server. It creates `config.json` and its `data` folder. The example JSON shipped in the archive is a reference; you do not need to rename or edit it.

## 2. Start and finish setup

Double-click `C:\PS4Server\ps4-server.exe`. A console window stays open. Running it with no arguments uses the EXE's folder for configuration and data. Leave this window open while using the server.

Copy the entire **Setup:** address printed in that window into your browser, including `#token=...`. The address resembles `http://localhost:8080/setup#token=...`; the actual token is unique. If Windows shows a publisher warning, check the download source before choosing to run the unsigned executable.

In the wizard:

1. Create an administrator username and a password of at least 12 characters.
2. Add these mappings using the directory picker. Create the folders in Explorer first; use only the rows you need.

   | ID | Folder | Category | Enabled |
   | --- | --- | --- | --- |
   | games | `D:\PS4Packages\Games` | games | Yes |
   | updates | `D:\PS4Packages\Updates` | updates | Yes |
   | dlc | `D:\PS4Packages\DLC` | dlc | Yes |

3. Set application data storage to `C:\PS4Server\data`. This stores catalogs, covers and caches, not copies of your packages.
4. Enter your PC's LAN address, for example **`http://192.168.1.20:8080`**. To find it, open Windows Settings → Network & internet → your connected network → Properties, and look for IPv4 address. Substitute your actual address everywhere below. Keep the main listener at `:8080`. `localhost` and `0.0.0.0` are not PS4-reachable public addresses.
5. Enable scanning at startup and optionally set the interval to **15 minutes**. Leave IGDB disabled initially if you do not have credentials; package scanning works without it.
6. Save, sign in, and let the first scan finish. Check the administration page for errors or unavailable folders.

The setup token expires after saving. On later launches, open `http://localhost:8080/library` or `http://localhost:8080/admin` directly. To stop the console server, press **Ctrl+C** in its window. Next time, double-click the same EXE again; your settings remain saved.

## 3. Allow home-network access

Set your trusted home network's Windows network profile to **Private**. If Windows asks about firewall access, allow the server on **Private networks**.

If another device cannot connect, open **PowerShell as administrator** (Start → type PowerShell → right-click → Run as administrator) and add this rule once:

```powershell
New-NetFirewallRule -DisplayName 'PS4 Games Server' -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080 -Profile Private -RemoteAddress LocalSubnet
```

This permits port 8080 from your local subnet on Private networks ([Microsoft firewall command reference](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule)). If you change the listener port, adjust the rule and public URL too. No router port forwarding is needed for devices on the same home network. Guest Wi-Fi/device isolation can prevent the PS4 from reaching your PC.

Test `http://192.168.1.20:8080/library` from another computer or phone on that network. If your PC's address changes, update Public URL in administration; a DHCP reservation in your router helps keep the address stable.

## 4. Add feed URLs to FPKGi

Open the library and use **Copy** beside each feed URL you want to use. For the example above, the `content_urls` portion of your FPKGi configuration is:

```json
{
  "schema": 1,
  "content_urls": {
    "games": "http://192.168.1.20:8080/feeds/games",
    "updates": "http://192.168.1.20:8080/feeds/updates",
    "dlc": "http://192.168.1.20:8080/feeds/dlc"
  }
}
```

Use your actual PC address. Merge `content_urls` into your existing FPKGi configuration, retaining other settings. Use the category URLs shown in your library when your folders or categories differ. The feeds and downloads are public on your LAN; only administration requires login. Real console installation remains an external acceptance check.

## Optional: start automatically as a Windows service

A service starts after Windows boots, continues after you sign out, and has no console window. Finish the wizard and verify normal EXE operation first. Use local disk folders for this example; services do not inherit your login's mapped network drives or network credentials.

1. Stop the console server with **Ctrl+C**. Keep the EXE in its permanent location, `C:\PS4Server`; the service records its full path.
2. Open **PowerShell as administrator** and run:

   ```powershell
   Set-Location C:\PS4Server
   .\ps4-server.exe service install
   ```

   Installation uses your existing `config.json` and remembers the current folder so relative data paths keep working. It refuses incomplete setup or an active writer. Installation registers the service; it does not start it yet.

3. Grant the built-in **Local Service** account access to the example folders:

   ```powershell
   icacls.exe 'C:\PS4Server' /grant '*S-1-5-19:(OI)(CI)M' /T
   icacls.exe 'D:\PS4Packages' /grant '*S-1-5-19:(OI)(CI)RX' /T
   ```

   The first command grants Modify access to server configuration/data/logs and the EXE folder. The second grants only Read/Execute access to packages. The numeric SID works on non-English Windows too ([Microsoft icacls reference](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/icacls)). Substitute your actual folders. If configuration, data, custom backgrounds or packages live elsewhere, grant the required access to those specific locations as well. Installation does not change folder permissions automatically.

4. Start it and check it:

   ```powershell
   .\ps4-server.exe service start
   .\ps4-server.exe service status
   .\ps4-server.exe health
   ```

   Expect `Service running.`, `running`, and `healthy`. Open `http://localhost:8080/admin`. You may close PowerShell. The service is named **PS4GamesServer**, displayed as **PS4 Games Server**, runs under the limited **Local Service** account, and uses Automatic (Delayed Start). It can take a short while to start after boot. You can also manage it through Windows **Services** (`services.msc`).

Useful commands, from elevated PowerShell in `C:\PS4Server`:

```powershell
# Restart after changing listeners or data storage:
.\ps4-server.exe service stop
.\ps4-server.exe service start

# Remove automatic startup (keeps configuration, data and packages):
.\ps4-server.exe service uninstall
```

To upgrade, stop the service, replace the EXE with the new Windows build, then start the service. Keep its path unchanged. Moving the EXE or changing its configuration location requires uninstalling and installing again. A custom configuration can be selected at installation with `service install -config 'C:\PS4Server\config.json'`; the other service actions always operate on the registered PS4GamesServer instance. Only one instance is installed by these commands.

Startup/shutdown errors are appended to `service.log` beside the configuration. Recent scan messages remain in the admin panel. If start fails, check this log, folder permissions, port conflicts, and that the console EXE is stopped. If the log cannot be created, check folder permissions and Windows Event Viewer → Windows Logs → System → Service Control Manager. Restarting the service also reloads its settings.

## Troubleshooting

- **The EXE window closes immediately:** open PowerShell in the extracted folder and run `.\ps4-server.exe` to read the error. An occupied port, malformed configuration or another running instance can prevent startup.
- **Browser works on the PC but PS4 cannot connect:** check the actual IPv4 address, Private firewall profile/rule, subnet and Wi-Fi isolation. Never put `localhost` into FPKGi on the PS4.
- **No packages:** verify the enabled mapping paths and scan status. The server accepts PS4 packages; encrypted metadata is reported without decryption.
- **Service cannot see a drive/folder:** use local paths and grant Local Service access. USB disks must be connected when scans run; unavailable mappings retain their previous entries and report an error.
- **Forgot the password:** stop the console/service, open PowerShell in `C:\PS4Server`, run `.\ps4-server.exe password-reset`, type a new password, then press Ctrl+Z and Enter. Restart afterwards. Terminal input may be visible.
- **Backups:** stop the server, then copy `config.json` and `data` to a safe location. Keep your package backups separately. Do not remove lock files while the server is running.
