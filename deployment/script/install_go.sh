apt install -y wget --fix-missing

wget -q https://golang.google.cn/dl/go1.15.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.15.linux-amd64.tar.gz
export PATH="/usr/local/go/bin:${PATH}"
go env -w GOPROXY=https://goproxy.io && go env -w GO111MODULE=on