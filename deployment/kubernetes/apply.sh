cd `dirname $0`

kubectl apply -f config/kmon.yaml -n ebpf-monitor
kubectl apply -f config/config.yaml -n ebpf-monitor