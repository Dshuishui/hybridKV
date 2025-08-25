#!/bin/bash
# 简化版Go环境配置脚本，添加解压缩和启动KV服务器功能
# 适用于Ubuntu系统
# 日期：2025-05-19

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 默认配置
GO_VERSION="1.19.5"
INSTALL_DIR="$HOME/local"
ZIP_PATH="$HOME/Hybrid_KV_store.zip"
DEFAULT_ADDRESS="192.168.1.74:3087"
DEFAULT_INTERNAL_ADDRESS="192.168.1.74:30871"
DEFAULT_PEERS="192.168.1.74:30871,192.168.1.72:30871,192.168.1.100:30871"

# 打印帮助信息
print_usage() {
    echo -e "${BLUE}Go语言环境自动配置与KV服务器启动脚本${NC}"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help              显示此帮助信息"
    echo "  -v, --version <版本>    指定Go版本 (默认: $GO_VERSION)"
    echo "  -d, --dir <目录>        指定安装目录 (默认: $INSTALL_DIR)"
    echo "  -z, --zip <路径>        指定Hybrid_KV_store.zip路径 (默认: $ZIP_PATH)"
    echo ""
    echo "示例:"
    echo "  $0                      # 使用默认配置安装"
    echo "  $0 -v 1.20.2            # 安装Go 1.20.2版本"
    echo "  $0 -d ~/goenv           # 指定安装目录为~/goenv"
    echo "  $0 -z ~/downloads/Hybrid_KV_store.zip  # 指定zip文件路径"
    echo ""
}

# 解析命令行参数
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help)
                print_usage
                exit 0
                ;;
            -v|--version)
                GO_VERSION="$2"
                shift 2
                ;;
            -d|--dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -z|--zip)
                ZIP_PATH="$2"
                shift 2
                ;;
            *)
                echo -e "${RED}错误: 未知选项 $1${NC}" >&2
                print_usage
                exit 1
                ;;
        esac
    done
}

# 设置环境变量函数 - 改进版，确保多种Shell配置文件都得到更新
setup_environment() {
    echo -e "6. 添加环境变量以实现永久生效..."
    
    # 添加到.bashrc（针对bash）
    BASH_CONFIG="$HOME/.bashrc"
    setup_config_file "$BASH_CONFIG"
    
    # 添加到.profile（更广泛的配置文件，许多系统会读取）
    PROFILE_CONFIG="$HOME/.profile"
    setup_config_file "$PROFILE_CONFIG"
    
    # 如果用户使用zsh，也添加到zsh配置
    ZSH_CONFIG="$HOME/.zshrc"
    if [ -f "$ZSH_CONFIG" ]; then
        setup_config_file "$ZSH_CONFIG"
    fi
    
    # 设置当前会话的环境变量
    export GOROOT="$INSTALL_DIR/go"
    export GOPATH="$INSTALL_DIR/gowork"
    export PATH="$PATH:$GOROOT/bin:$GOPATH/bin"
    export GOPROXY="https://goproxy.io,direct"
    
    echo -e "${YELLOW}提示：环境变量已配置为永久生效${NC}"
    echo -e "您可以通过以下几种方式使环境变量在当前会话立即生效："
    echo -e "1. 执行 ${BLUE}source $HOME/.bashrc${NC} (如果您使用bash)"
    echo -e "2. 执行 ${BLUE}source $HOME/.zshrc${NC} (如果您使用zsh)"
    echo -e "3. 重新登录系统"
    echo -e "在下次登录时，这些设置将自动生效。"
}

# 为指定的配置文件添加环境变量
setup_config_file() {
    local CONFIG_FILE="$1"
    echo -e "   处理配置文件: ${GREEN}$CONFIG_FILE${NC}"
    
    # 确保配置文件存在
    touch "$CONFIG_FILE"

    # 检查是否已经添加过环境变量
    if grep -q "export GOROOT=$INSTALL_DIR/go" "$CONFIG_FILE"; then
        echo -e "   Go环境变量已存在于 $CONFIG_FILE 中"
        
        # 询问是否要更新
        read -p "   是否要更新环境变量? [y/N] " -n 1 -r UPDATE_ENV
        echo
        if [[ $UPDATE_ENV =~ ^[Yy]$ ]]; then
            # 删除旧的环境变量设置
            sed -i '/# Go环境变量配置/,+5d' "$CONFIG_FILE"
            echo -e "   ${GREEN}已删除旧的环境变量配置${NC}"
        else
            echo -e "   ${YELLOW}保留现有环境变量配置${NC}"
            return
        fi
    fi

    # 添加环境变量
    echo "" >> "$CONFIG_FILE"
    echo "# Go环境变量配置" >> "$CONFIG_FILE"
    echo "export GOROOT=$INSTALL_DIR/go" >> "$CONFIG_FILE"
    echo "export GOPATH=$INSTALL_DIR/gowork" >> "$CONFIG_FILE"
    echo "export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin" >> "$CONFIG_FILE"
    echo "export GOPROXY=https://goproxy.io,direct" >> "$CONFIG_FILE"
    
    echo -e "   ${GREEN}环境变量已添加到 $CONFIG_FILE${NC}"
}

# 解压缩并启动KV服务器
extract_and_run_kv() {
    echo -e "${BLUE}7. 解压并准备启动KV服务器...${NC}"

    # 检查压缩包是否存在
    if [ ! -f "$ZIP_PATH" ]; then
        echo -e "${RED}错误: 找不到Hybrid_KV_store.zip压缩包，请检查路径: $ZIP_PATH${NC}"
        read -p "是否继续而不启动KV服务器? [y/N] " -n 1 -r CONTINUE
        echo
        if [[ ! $CONTINUE =~ ^[Yy]$ ]]; then
            return 1
        else
            return 0
        fi
    fi

    # 解压缩到当前目录
    echo -e "   正在解压 Hybrid_KV_store.zip ..."
    unzip -o "$ZIP_PATH" -d "$INSTALL_DIR"
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}解压缩失败，请检查压缩包格式或权限${NC}"
        return 1
    fi
    
    # 获取解压后的目录名称（假设解压后会创建一个名为Hybrid_KV_store的目录）
    KV_DIR="$INSTALL_DIR/Hybrid_KV_store"
    if [ ! -d "$KV_DIR" ]; then
        # 尝试使用通配符查找解压后的目录
        KV_DIR=$(find "$INSTALL_DIR" -type d -name "*KV*" -print -quit)
        if [ -z "$KV_DIR" ]; then
            echo -e "${RED}无法确定解压后的KV存储目录${NC}"
            return 1
        fi
    fi
    
    echo -e "   KV存储目录: ${GREEN}$KV_DIR${NC}"
    
    # 获取用户自定义参数
    echo -e "${BLUE}8. 配置KV服务器参数...${NC}"
    echo -e "   请输入以下参数（留空使用默认值）:"
    
    read -p "   外部地址 (默认: $DEFAULT_ADDRESS): " ADDRESS
    ADDRESS=${ADDRESS:-$DEFAULT_ADDRESS}
    
    read -p "   内部地址 (默认: $DEFAULT_INTERNAL_ADDRESS): " INTERNAL_ADDRESS
    INTERNAL_ADDRESS=${INTERNAL_ADDRESS:-$DEFAULT_INTERNAL_ADDRESS}
    
    read -p "   节点列表 (默认: $DEFAULT_PEERS): " PEERS
    PEERS=${PEERS:-$DEFAULT_PEERS}
    
    # 切换到KV目录并启动服务器
    echo -e "${BLUE}9. 准备启动KV服务器...${NC}"
    echo -e "   使用以下参数启动:"
    echo -e "   - 外部地址: ${GREEN}$ADDRESS${NC}"
    echo -e "   - 内部地址: ${GREEN}$INTERNAL_ADDRESS${NC}"
    echo -e "   - 节点列表: ${GREEN}$PEERS${NC}"
    
    # 检查运行文件是否存在
    SERVER_FILE="$KV_DIR/kvstore/kvserver/kvserver.go"
    if [ ! -f "$SERVER_FILE" ]; then
        echo -e "${RED}错误: 找不到服务器文件 $SERVER_FILE${NC}"
        echo -e "${YELLOW}请检查解压后的目录结构，或手动查找正确的文件路径${NC}"
        return 1
    fi
    
    echo -e "${YELLOW}按Ctrl+C可以随时终止服务器${NC}"
    echo -e "${BLUE}正在启动KV服务器...${NC}"
    
    # 使用cd命令切换到KV目录并启动服务器
    cd "$KV_DIR"
    go run kvstore/kvserver/kvserver.go -address "$ADDRESS" -internalAddress "$INTERNAL_ADDRESS" -peers "$PEERS"
}

# 主函数
main() {
    # 解析命令行参数
    parse_arguments "$@"
    
    echo -e "${BLUE}开始安装Go环境...${NC}"

    # 1. 创建安装目录
    echo -e "1. 创建安装目录..."
    mkdir -p "$INSTALL_DIR"
    cd "$INSTALL_DIR"

    # 2. 获取当前路径
    CURRENT_PATH=$(pwd)
    echo -e "   当前路径: ${GREEN}$CURRENT_PATH${NC}"

    # 3. 下载并解压Go，阿里云镜像
    echo -e "3. 下载并安装Go ${GO_VERSION}..."
    GO_DOWNLOAD_URL="https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-amd64.tar.gz"
    
    if [ -d "$INSTALL_DIR/go" ]; then
        echo -e "${YELLOW}检测到已存在Go安装，将先删除...${NC}"
        rm -rf "$INSTALL_DIR/go"
    fi
    
    echo -e "   正在下载Go ${GO_VERSION}..."
    wget -c "$GO_DOWNLOAD_URL" -O - | tar -xz -C "$INSTALL_DIR"

    # 4. 检查Go是否安装成功
    if [ -x "$INSTALL_DIR/go/bin/go" ]; then
        GO_VERSION_INSTALLED=$("$INSTALL_DIR/go/bin/go" version)
        echo -e "   Go安装成功: ${GREEN}$GO_VERSION_INSTALLED${NC}"
    else
        echo -e "${RED}Go安装失败，请检查下载链接和权限${NC}"
        exit 1
    fi

    # 5. 创建gowork目录
    echo -e "5. 创建GOPATH目录结构..."
    mkdir -p "$INSTALL_DIR/gowork/src" "$INSTALL_DIR/gowork/bin" "$INSTALL_DIR/gowork/pkg"

    # 6. 添加环境变量
    setup_environment

    echo -e "${GREEN}Go环境配置完成!${NC}"
    
    # 不再自动执行source，而是给出明确的使用提示
    echo -e "${YELLOW}环境变量已配置，但需要您手动使其在当前会话中生效：${NC}"
    echo -e "请执行 ${BLUE}source $HOME/.bashrc${NC} 使环境变量在当前会话立即生效"
    echo -e "或重新登录系统以应用这些更改"
    
    # 询问用户是否要在当前会话中应用设置
    read -p "是否立即应用环境变量到当前会话? [Y/n] " -n 1 -r APPLY_NOW
    echo
    if [[ ! $APPLY_NOW =~ ^[Nn]$ ]]; then
        echo -e "${YELLOW}正在应用环境变量到当前会话...${NC}"
        source "$HOME/.bashrc"
        echo -e "环境变量已应用，测试Go安装: ${GREEN}$(go version)${NC}"
    else
        echo -e "${YELLOW}环境变量将在下次登录时生效，或执行 source 命令手动生效${NC}"
    fi
    
    # 询问是否要继续启动KV服务器
    read -p "是否要继续解压并启动KV服务器? [Y/n] " -n 1 -r START_KV
    echo
    if [[ ! $START_KV =~ ^[Nn]$ ]]; then
        # 7. 解压缩和启动KV服务器
        extract_and_run_kv
        
        # 如果KV服务器启动失败，给出手动启动的提示
        if [ $? -ne 0 ]; then
            echo -e "${YELLOW}若要手动启动KV服务器，请执行以下命令:${NC}"
            echo -e "${BLUE}cd 解压后的目录${NC}"
            echo -e "${BLUE}go run kvstore/kvserver/kvserver.go -address <地址> -internalAddress <内部地址> -peers <节点列表>${NC}"
        fi
    else
        echo -e "${YELLOW}已跳过KV服务器部分，安装完成${NC}"
    fi
    
    # 创建启动脚本
    create_startup_script
}

# 创建一个单独的启动脚本
create_startup_script() {
    echo -e "${BLUE}创建KV服务器启动脚本...${NC}"
    STARTUP_SCRIPT="$INSTALL_DIR/start_kv_server.sh"
    
    cat > "$STARTUP_SCRIPT" << EOL
#!/bin/bash
# KV服务器启动脚本

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 设置Go环境变量
export GOROOT="$INSTALL_DIR/go"
export GOPATH="$INSTALL_DIR/gowork"
export PATH="\$PATH:\$GOROOT/bin:\$GOPATH/bin"
export GOPROXY="https://goproxy.io,direct"

# 默认参数
DEFAULT_ADDRESS="$DEFAULT_ADDRESS"
DEFAULT_INTERNAL_ADDRESS="$DEFAULT_INTERNAL_ADDRESS"
DEFAULT_PEERS="$DEFAULT_PEERS"

# 使用参数或默认值
ADDRESS="\${1:-\$DEFAULT_ADDRESS}"
INTERNAL_ADDRESS="\${2:-\$DEFAULT_INTERNAL_ADDRESS}"
PEERS="\${3:-\$DEFAULT_PEERS}"

echo -e "\${BLUE}启动KV服务器...\${NC}"
echo -e "使用以下参数:"
echo -e "- 外部地址: \${GREEN}\$ADDRESS\${NC}"
echo -e "- 内部地址: \${GREEN}\$INTERNAL_ADDRESS\${NC}"
echo -e "- 节点列表: \${GREEN}\$PEERS\${NC}"

# 切换到KV目录并启动服务器
cd "$INSTALL_DIR/Hybrid_KV_store" || {
    echo -e "\${YELLOW}尝试查找KV目录...\${NC}"
    KV_DIR=\$(find "$INSTALL_DIR" -type d -name "*KV*" -print -quit)
    if [ -z "\$KV_DIR" ]; then
        echo -e "\${RED}无法找到KV目录，请检查安装\${NC}"
        exit 1
    fi
    cd "\$KV_DIR" || exit 1
}

echo -e "\${YELLOW}按Ctrl+C可以随时终止服务器\${NC}"
go run kvstore/kvserver/kvserver.go -address "\$ADDRESS" -internalAddress "\$INTERNAL_ADDRESS" -peers "\$PEERS"
EOL

    # 设置脚本为可执行
    chmod +x "$STARTUP_SCRIPT"
    
    echo -e "${GREEN}KV服务器启动脚本已创建: $STARTUP_SCRIPT${NC}"
    echo -e "您可以使用以下命令启动KV服务器:"
    echo -e "${BLUE}$STARTUP_SCRIPT${NC} - 使用默认参数"
    echo -e "或"
    echo -e "${BLUE}$STARTUP_SCRIPT <地址> <内部地址> <节点列表>${NC} - 使用自定义参数"
}

# 执行主函数
main "$@"