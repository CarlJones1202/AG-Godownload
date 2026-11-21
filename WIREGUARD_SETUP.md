# WireGuard VPN Configuration

This application supports selective routing of scraper requests through a WireGuard VPN tunnel (e.g., Proton VPN) to bypass regional blocks.

## Setup Instructions

### 1. Obtain Your WireGuard Configuration

1. Log in to your Proton VPN account at https://account.protonvpn.com
2. Navigate to **Downloads** → **WireGuard configuration**
3. Create a new configuration:
   - **Name**: Give it a descriptive name (e.g., "AG-Godownload")
   - **Platform**: Select "Router" or similar (for third-party clients)
   - **Server**: Choose a server location (preferably outside Louisiana/restricted regions)
4. Click **Create** and then **Download**
5. Save the `.conf` file

### 2. Place the Configuration File

Place your downloaded WireGuard configuration file in **one of these locations**:

**Option A (Recommended)**: Root of the project directory
```
<project-root>\wireguard.conf
```
This file is gitignored because it contains a private key. Keep it local and do not
commit it.

**Option B**: Specify a custom location via environment variable
```powershell
$env:WIREGUARD_CONF = "C:\path\to\your\config.conf"
```

### 3. Restart the Application

The tunnel is created on demand, the first time a request is made to one of the routed
domains below. If no such request is made, the config is never loaded. Set
`BYPASS_VPN=true` in the environment to force all requests onto the direct connection
for testing.

## How It Works

The application automatically routes requests to the following domains through the WireGuard tunnel:

- metart.com
- playboy.com
- femjoy.com
- met-art.com
- sexart.com
- thelifeerotic.com
- eternaldesire.com
- mplstudios.com

All other requests use your normal internet connection. This ensures:
- ✅ Only scraper traffic is routed through the VPN
- ✅ No system-wide VPN configuration needed
- ✅ Other applications are unaffected
- ✅ Portable across machines (just copy the .conf file)

## Verification

On the first request to a routed domain, you should see log messages like:
```
[VPN] URL requires WireGuard tunnel: https://www.metart.com/...
Loading WireGuard configuration from: wireguard.conf
WireGuard tunnel established successfully
```
Colly-based scrapers log `Using WireGuard tunnel for Colly request to: <url>` instead.

## Troubleshooting

**"WireGuard config not found"**
- Ensure the file is named `wireguard.conf` and placed in the project root
- Or set the `WIREGUARD_CONF` environment variable to the correct path

**"Failed to create WireGuard dialer"**
- Verify the `.conf` file is valid and not corrupted
- Try downloading a fresh configuration from Proton VPN

**Requests still blocked**
- Check that the domain is in the blocked domains list (see `wireguard_service.go`)
- Verify your VPN server location is outside the restricted region
