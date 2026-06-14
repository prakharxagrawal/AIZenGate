# ZenGate AI — Production Hardening & Cloudflare Deployment Runbook

This runbook outlines steps to deploy ZenGate AI to production, configure Cloudflare Tunnels for secure edge routing with SSL/TLS, and establish network security rules.

---

## 🔒 1. Edge Security with Cloudflare Tunnel

Cloudflare Tunnel (`cloudflared`) connects your local server directly to the Cloudflare network without opening public inbound ports (like `80` or `443`) on your local router or cloud firewall.

### Prerequisites
1. A registered domain active on your Cloudflare account.
2. The `cloudflared` daemon installed on your target deployment machine.

### Setup Steps

1. **Authenticate Cloudflare CLI**
   Run the login command on your VM:
   ```bash
   cloudflared tunnel login
   ```
   This opens a browser link to authorize the daemon. Select your target domain.

2. **Create the Tunnel**
   Create a new tunnel instance (e.g., named `zengate-production`):
   ```bash
   cloudflared tunnel create zengate-production
   ```
   This generates a unique tunnel ID and saves credentials in a `.json` file.

3. **Configure the Route Mapping**
   Create a configuration file at `~/.cloudflared/config.yml`:
   ```yaml
   tunnel: <TUNNEL_UUID>
   credentials-file: /home/ubuntu/.cloudflared/<TUNNEL_UUID>.json

   ingress:
     # Map public subdomain to the local gateway service port
     - hostname: zengate.yourdomain.com
       service: http://localhost:8080
     # Fallback catch-all returns 404
     - service: http_status:404
   ```

4. **Associate CNAME DNS Records**
   Route your public hostname domain traffic through the tunnel:
   ```bash
   cloudflared tunnel route dns zengate-production zengate.yourdomain.com
   ```

5. **Run the Tunnel as a System Service**
   Install the tunnel service to automatically launch on system boot:
   ```bash
   sudo cloudflared service install
   sudo systemctl start cloudflared
   sudo systemctl enable cloudflared
   ```

---

## 🛠️ 2. Oracle Cloud VM Infrastructure Networking

If deploying on Oracle Cloud Infrastructure (OCI) Free Tier, you must authorize traffic rules:

1. **OCI VCN Security Lists**
   Navigate to *Compute Instance -> Primary VNIC -> Subnet -> Security List*. Add an **Ingress Rule**:
   - **Source CIDR:** `0.0.0.0/0` (or Cloudflare IP ranges only if bypassing the tunnel and exposing directly)
   - **IP Protocol:** `TCP`
   - **Destination Port Range:** `8080` (for direct health testing) or rely completely on Cloudflare Tunnel (no open ingress ports needed!)

2. **VM Local Firewall Rules**
   Configure local iptables/firewalld rules on the VM to accept forwarding on port `8080`:
   ```bash
   # For Ubuntu/Debian VMs:
   sudo ufw allow 8080/tcp
   sudo ufw reload
   ```

---

## 📈 3. Monitoring Infrastructure Configuration

To maintain reliability, configure Prometheus to scrape gateway statistics and mount them to Grafana:

1. **Scraping Configuration**
   Ensure `prometheus.yml` matches target deployment addresses:
   ```yaml
   scrape_configs:
     - job_name: 'zengate-gateway'
       scrape_interval: 5s
       static_configs:
         - targets: ['localhost:8080']
   ```

2. **Grafana Dashboards Provisioning**
   On startup, ZenGate is configured to provision dashboards automatically.
   - Prometheus datasource is loaded via `deploy/grafana/provisioning/datasources/datasource.yml`.
   - ZenGate metrics are rendered using the dashboard configuration at `deploy/grafana/dashboards/zengate.json`.
