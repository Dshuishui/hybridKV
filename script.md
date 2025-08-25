install Go:
```bash
wget https://studygolang.com/dl/golang/go1.15.4.linux-amd64.tar.gz
sudo tar -zxvf go1.15.4.linux-amd64.tar.gz -C ~/
mkdir ~/Go
echo "export GOPATH=~/Go" >>  ~/.bashrc 
echo "export GOROOT=~/go" >> ~/.bashrc 
echo "export GOTOOLS=\$GOROOT/pkg/tool" >> ~/.bashrc
echo "export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin" >> ~/.bashrc
source ~/.bashrc
go env -w GOPROXY=https://goproxy.cn
```
git push fail:
```bash
git config --global http.proxy http://127.0.0.1:1080
git config --global https.proxy http://127.0.0.1:1080
git config --global --unset http.proxy
git config --global --unset https.proxy
```

start kvserver cluster: 
`go run kvstore/kvserver/kvserver.go -address 192.168.10.120:3088 -internalAddress 192.168.10.120:30881 -peers 192.168.10.120:30881,192.168.10.121:30881,192.168.10.122:30881`

kvserver with tcp and rpc:
`go run kvstore/kvserver/kvserver.go -address 192.168.10.120:3088 -tcpAddress 192.168.10.120:50000 -internalAddress 192.168.10.120:30881 -peers 192.168.10.120:30881`

start kvclient:
* RequestRatio benchmark: 
    `go run ./benchmark/hydis/benchmark.go -cnums 1 -mode RequestRatio -onums 100 -getratio 4 -servers 192.168.10.120:3088,192.168.10.121:3088,192.168.10.122:3088`
    * cums: number of clients, goroutines simulate
    * onums: number of operations, each client will do onums operations
    * mode: only support RequestRatio (put/get ratio is changeable)
    * getRatio: get times per put time
    * servers: kvserver address
    operation times = cnums * onums * (1+getRatio)

* benchmark from csv:
    `go run ./benchmark/hydis/benchmark.go -cnums 5 -mode BenchmarkFromCSV -servers benchmark001:3088,benchmark002:3088,benchmark003:3088,benchmark004:3088,benchmark005:3088`

* WASM Client by Rust:
    Need WASM Runtime(wasmedge...)
    `cd ./benchmark/wasm_client/rust-client`
    compile: `cargo build --example=tcpclient --target=wasm32-wasi`
    run: `wasmedge ./target/wasm32-wasi/debug/examples/tcpclient.wasm`