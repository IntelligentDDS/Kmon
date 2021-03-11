cd `dirname $0`

kubectl delete -f config/kmon.yaml -n ebpf-monitor
kubectl delete -f config/config.yaml -n ebpf-monitor