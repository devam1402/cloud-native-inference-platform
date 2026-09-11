# P3.6 — Single H100 GPU Sharing (planned, not yet started)

Goal: demonstrate GPU sharing techniques on the single rented H100,
proving each in isolation and restoring exclusive access between them.

## Order (deliberately NOT the naive MIG-first order)

1. **Time-Slicing** — do first. Pure device-plugin config, no driver
   mode change, zero risk, easiest to verify/rollback.
2. **MPS** — do second. Needs an MPS control daemon on the host but no
   GPU hardware-partitioning mode change. Moderate risk.
3. **MIG** — do last, and only after confirming the provider allows it:
       nvidia-smi -mig -h
       sudo nvidia-smi -mig 1
   Many cloud/rental providers disable MIG entirely or require a GPU
   reset/reboot for the mode change to take effect — check permission
   BEFORE spending time on profile/config work that depends on it.

## 1. Time-Slicing (implementation sketch)

Create a ConfigMap the device plugin reads for slice count:

    kubectl create configmap time-slicing-config -n kube-system \
      --from-literal=time-slicing.yaml='
    version: v1
    sharing:
      timeSlicing:
        resources:
        - name: nvidia.com/gpu
          replicas: 4
    '

Then patch the nvidia-device-plugin-daemonset to add --config-file and
mount this ConfigMap (exact manifest TBD — needs the current device
plugin version's config-file flag/mount convention verified before
applying).

Verify: nvidia.com/gpu capacity should show 4 (not 1); schedule 2+
concurrent Pods requesting nvidia.com/gpu:1 each and confirm they run
simultaneously (not queued for the single physical GPU); use
nvidia-smi from one Pod to observe the other's process in the shared
GPU's process list.

## 2. MPS (not yet designed in detail)

Needs an MPS control daemon lifecycle on the host and Pods requesting
a distinct resource type from the device plugin's MPS mode. Needs
researching the exact current config for the installed device-plugin
version before writing commands.

## 3. MIG (gated on the permission check above)

If allowed: inspect supported profiles (`nvidia-smi mig -lgip`), create
a profile, reconfigure the device plugin for MIG-aware resource names
(nvidia.com/mig-<profile>), schedule Pods against specific MIG
instances, verify isolation between them, capture evidence, then
restore the H100 to non-MIG exclusive mode
(`sudo nvidia-smi -mig 0`) before moving on.

## Baseline (captured pre-sharing)

See baseline/pre-sharing-capacity.txt — nvidia.com/gpu:1, driver
580.126.20, CUDA 13.0, H100 80GB, confirmed via a real nvidia-smi run
inside a Pod (see the main README.md in this directory).
