# Day-0 bootstrap

ArgoCD cannot deploy itself before it exists — this is a one-time manual
step, not a permanent exception to GitOps. Run once per fresh cluster:

    kubectl create namespace argocd
    kubectl apply -n argocd --server-side --force-conflicts \
      -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

Tracking `stable` (not a pinned version) is a deliberate choice for this
lab/portfolio cluster, which is torn down and rebuilt between sessions —
pin an exact version if this cluster becomes long-lived.

After install, apply the platform project and root application:

    kubectl apply -f gitops/projects/platform-project.yaml
    kubectl apply -f gitops/app-of-apps/root-application.yaml

Everything after that point is managed by ArgoCD from `gitops/apps/`.
