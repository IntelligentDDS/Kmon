export REGISTRY=harbor.dds-sysu.tech
docker login -u 595859893@qq.com -p a123456789B $REGISTRY
docker build -t ebpf_monitor:latest .
docker tag ebpf_monitor:latest $REGISTRY/ebpf_monitor/ebpf_monitor:latest
docker push $REGISTRY/ebpf_monitor/ebpf_monitor:latest