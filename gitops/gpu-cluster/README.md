# GPU worker cluster (for Kueue MultiKueue)

A standalone k3s cluster on rented GPU hardware, meant to be registered
as a MultiKueue worker cluster from cnip-gke — NOT joined as a node into
GKE (standard GKE does not support joining external nodes; that plan was
tried and correctly abandoned — GKE's control plane only issues
node-bootstrap credentials to VMs it provisions itself).

Provider used: JarvisLabs (an H100 80GB). Any provider offering full
root SSH access works the same way.

## Provisioning a fresh box

    curl -sfL https://get.k3s.io | sh -
    sleep 20
    kubectl get nodes   # should show Ready

If the node is stuck NotReady with "cni plugin not initialized" and it
doesn't clear after ~1 minute, don't deep-debug it — just reinstall clean:

    sudo /usr/local/bin/k3s-uninstall.sh
    curl -sfL https://get.k3s.io | sh -

k3s ships nvidia/nvidia-experimental RuntimeClasses and an nvidia
containerd runtime entry out of the box — no manual nvidia-ctk config
needed (nvidia-ctk's default docs target system containerd's
/etc/containerd/conf.d/, a path k3s's embedded containerd does not read).

## Exposing the GPU to Kubernetes

    kubectl apply -f nvidia-runtimeclass.yaml
    kubectl apply -f nvidia-device-plugin.yaml
    kubectl get nodes -o jsonpath='{.items[0].status.capacity}'
    # should show "nvidia.com/gpu":"1"

## Verify

    kubectl run gpu-test --image=nvidia/cuda:12.4.0-base-ubuntu22.04 \
      --restart=Never --overrides='{"spec":{"runtimeClassName":"nvidia","containers":[{"name":"gpu-test","image":"nvidia/cuda:12.4.0-base-ubuntu22.04","command":["nvidia-smi"],"resources":{"limits":{"nvidia.com/gpu":"1"}}}]}}'
    kubectl logs gpu-test
    kubectl delete pod gpu-test

## Exposing the k3s API server externally (required for MultiKueue)

By default k3s's TLS certificate only covers 127.0.0.1/localhost/internal
IPs — connecting from outside the box (which MultiKueue on cnip-gke
needs to do) fails TLS validation until the public IP is added as a SAN.
k3s does NOT regenerate its cert on a config change alone; the dynamic
cert file has to be deleted to force regeneration.

    echo 'tls-san:
      - "<PUBLIC_IP>"' | sudo tee /etc/rancher/k3s/config.yaml
    sudo systemctl stop k3s
    sudo rm -f /var/lib/rancher/k3s/server/tls/dynamic-cert.json
    sudo systemctl start k3s

Verify the cert now covers the public IP:

    echo | openssl s_client -connect <PUBLIC_IP>:6443 2>/dev/null | \
      openssl x509 -noout -text | grep -A2 "Subject Alternative Name"

Then export a usable external kubeconfig (the default one points at
127.0.0.1, which only works from inside the box):

    cat ~/.kube/config | sed 's|127.0.0.1|<PUBLIC_IP>|' > ~/kubeconfig-external.yaml

Copy it off the box from wherever needs to reach this cluster
(a dev machine, or eventually a Secret on cnip-gke for MultiKueue):

    scp -i <ssh-key> ubuntu@<PUBLIC_IP>:~/kubeconfig-external.yaml ./gpu-cluster-kubeconfig.yaml

Note: this provider (JarvisLabs) appears to preserve disk/cluster state
across stop/restart but assigns a new hostname and IP each time — the
tls-san and kubeconfig steps above will need repeating after any restart
with the new IP.
