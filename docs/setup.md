# Beginner setup

Download and unpack the release for your operating system. No Go, Node.js, Python or Docker is required for native use.

1. On Windows, double-click `ps4-server.exe`. On Linux/macOS, run `./ps4-server` in a terminal. Leave it running.
2. Open the full **Setup** URL printed in the window, including `#token=...`. On another computer, substitute the server's address for `localhost`.
3. Create your admin account, choose your package folder and writable data folder, and enter the LAN URL your PS4 can reach (for example `http://192.168.1.20:8080`). IGDB is optional.
4. Save, sign in and start a scan. Open `/library` to browse games, descriptions and screenshots. Copy the Games feed URL into **PS4 Games Installer → Options → Settings → Games JSON URL**, then test and save it.

Full illustrated instructions, a NAS Compose example and the folder layout are in the [beginner README](https://github.com/e1z0/PS4-Games-Server/blob/main/README.md).

- [Windows EXE, firewall and automatic service](https://github.com/e1z0/PS4-Games-Server/blob/main/WINDOWS.md)
- [Expert settings, IGDB, extra folders, backups and troubleshooting](https://github.com/e1z0/PS4-Games-Server/blob/main/EXPERT.md)
- [Developer builds, tests and releases](https://github.com/e1z0/PS4-Games-Server/blob/main/DEVELOPER.md)

Configuration and generated materials are saved separately from packages. Keep your PC/NAS running while the PS4 downloads. The server and package/artwork URLs must be reachable from the console; administration requires login, feeds/downloads do not.
