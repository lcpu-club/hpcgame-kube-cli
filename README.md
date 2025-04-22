# HPCGame Kube CLI

HPCGame kube 平台的命令行工具，提供与平台交互的功能。可以快速配置环境、创建容器、管理文件和连接容器。它简化了 Kubernetes 的操作流程，让能够专注于比赛任务而非基础设施管理。

## 主要功能

- 安装 kubectl、vscode插件
- 简化容器创建
- 提供命令行的容器交互
- 端口转发

## 教程

### 安装

从[ releases](https://github.com/lcpu-club/hpcgame-kube-cli/releases) 页面下载最新版本的二进制文件，解压后将其放置在 PATH 中。

选择适合操作系统的压缩包

- Linux: hpcgame-linux-amd64.tar.gz 或 hpcgame-linux-arm64.tar.gz
- macOS: hpcgame-darwin-amd64.tar.gz 或 hpcgame-darwin-arm64.tar.gz
- Windows: hpcgame-windows-amd64.zip

### 使用流程

要获取任何命令的详细帮助，请使用：
```bash
hpcgame <命令> --help
```
#### 初始化环境

首先，需要初始化环境。如果您使用vscode remote连接到linux服务器，并在其上开发，可以直接在vscode terminal中执行如下命令。

```bash
hpcgame install
```
这个命令会：

- 检查并安装 kubectl（如果需要）
- 提示输入比赛平台提供的 kubeconfig
- 验证并保存 kubeconfig
- 安装必要的 VSCode 扩展（如果可用）
- 显示可用的计算分区信息

#### 创建容器

创建一个新的容器：
```bash
hpcgame create --partition=cpu --cpu=4 --memory=8 --gpu=1 --image=ubuntu:24.04 --name=my-container
```

也可以使用短命令格式：
```bash
hpcgame create -p gpu -c 2 -m 4 -g 1 -i ubuntu:24.04 -n my-container
```
参数说明：

```
-p, --partition: 分区名称（如 cpu, gpu 等）
-c, --cpu: CPU 核心数
-m, --memory: 内存大小（GiB）
-g, --gpu: GPU 数量
-i, --image: 容器镜像
-n, --name: 容器名称（默认自动生成）
```

如果您省略任何必要参数，工具将交互式地请求输入。

#### 查看容器
列出容器和可用分区：
```bash
hpcgame ls
hpcgame lspart
```

#### 连接到容器
打开容器的 shell 终端：
```bash
hpcgame shell <容器名称>
```

#### 在容器中执行命令
在不进入 shell 的情况下执行命令：
```bash
hpcgame exec <容器名称> <命令>
```
例如：
```bash
hpcgame exec my-container ls -la
```

#### 传输文件

在本地和容器之间复制文件：

从本地到容器：
```bash
hpcgame cp ./local-file.txt my-container:/path/to/destination/
```
从容器到本地：
```bash
hpcgame cp my-container:/path/to/file.txt ./local-destination/
```

#### 端口转发
将容器端口映射到本地端口：

```bash
hpcgame portforward my-container 8080:80
```        
这会将容器的 80 端口映射到本地的 8080 端口。

#### 删除容器
删除不再需要的容器：
```bash
hpcgame delete <容器名称>
```

#### 注意事项

- 各分区对 CPU, 内存和 GPU 有资源限制
- 文件传输和连接操作需要容器处于运行状态

## 问题反馈

如有问题或建议，请在 GitHub Issues 页面提交反馈。