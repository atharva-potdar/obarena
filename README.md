# IICPC Summer Hackathon 2026

## Prerequisites

### Dev Environment

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--write-kubeconfig-mode 644" sh -
echo 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml' >> ~/.bashrc
source ~/.bashrc
wget https://storage.googleapis.com/gvisor/releases/release/latest/x86_64/runsc -P /tmp
wget https://storage.googleapis.com/gvisor/releases/release/latest/x86_64/containerd-shim-runsc-v1 -P /tmp
sudo mv /tmp/runsc /usr/bin/runsc
sudo mv /tmp/containerd-shim-runsc-v1 /usr/bin/containerd-shim-runsc-v1
sudo chmod 755 /usr/bin/runsc
sudo chmod 755 /usr/bin/containerd-shim-runsc-v1
sudo mkdir -p /var/lib/rancher/k3s/agent/etc/containerd
sleep 5
sudo cp /var/lib/rancher/k3s/agent/etc/containerd/config.toml \
        /var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl
sudo tee -a /var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl <<'EOF'

[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runsc]
  runtime_type = "io.containerd.runsc.v1"
EOF
sudo systemctl restart k3s
kubectl apply -f - <<'EOF'
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
EOF
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4 | bash
sudo pacman -S just
just
```
