package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	kubeconfigDir  = ".hpcgame"
	kubeconfigFile = "kubeconfig"
)

type Container struct {
	Name   string
	CPU    int
	Memory string
	GPU    int
}

type Partition struct {
	Name        string
	Description string
	GPUTag      string
	GPUName     string
	Images      []string
	CPULimit    int
	MemoryLimit int // in GiB
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := os.Args[1]

	switch command {
	case "install":
		install()
	case "help":
		printHelp()
	case "create":
		createContainer()
	case "ls":
		listContainers()
	case "lspart":
		listPartitions(getPartitions())
	case "delete":
		deleteContainer()
	case "shell":
		shellContainer()
	case "exec":
		execInContainer()
	case "cp":
		copyFiles()
	case "portforward":
		portForward()
	default:
		fmt.Printf("未知命令: %s\n", command)
		printHelp()
	}
}

func printHelp() {
	helpText := `HPCGame CLI 工具

用法:
  hpcgame <命令> [参数]

可用命令:
  install		安装并配置必要的组件
  ls			列出当前账号的所有容器
  lspart		列出可用的分区
  shel			连接到指定容器的终端
  exec			在指定容器中执行命令
  cp			在本地和容器之间复制文件
  create		创建一个新的容器
  delete		删除指定的容器
  portforward	设置本地端口到容器端口的转发
  help			显示帮助信息

install 命令:
  检查并安装kubectl、配置kubeconfig，并为VSCode安装必要的扩展

ls 命令:
  显示当前账号拥有的所有容器，以及可用的分区信息

lspart 命令:
  显示可用的分区信息

shell 命令:
  连接到指定的容器的终端
  用法: hpcgame shell <容器名称>

exec 命令:
  在指定的容器中执行命令
  用法: hpcgame exec <容器名称> <命令>

cp 命令:
  在本地和容器之间复制文件
  用法: hpcgame cp <源文件> <目标文件>
  例如: hpcgame cp ./local-file.txt my-container:/path/to/file.txt
        hpcgame cp my-container:/path/to/file.txt ./local-file.txt

create 命令:
  创建一个新的容器
  参数:
    选项:
    -p, --partition string   指定分区名称
    -c, --cpu int            指定CPU核心数
    -m, --memory int         指定内存大小，单位GiB
    -g, --gpu int            指定GPU数量 (默认为0)
    -i, --image string       指定容器镜像
    -n, --name string        指定容器名称 (默认自动生成)
    -h, --help               显示帮助信息
  
  示例:
    # 创建一个有4个CPU核心、8GiB内存的容器在cpu分区
    hpcgame create --partition=x86 --cpu=4 --memory=8
    
    # 创建一个指定镜像和名称的容器
    hpcgame create -p cpu -c 1 -i ubuntu:20.04 -n my-container

delete 命令:
  删除指定的容器
  用法: hpcgame delete <容器名称>

portforward 命令:
  设置本地端口到容器端口的转发
  用法: hpcgame portforward <容器名称> <本地端口>:<容器端口>
  例如: hpcgame portforward my-container 8080:80
`
	fmt.Println(helpText)
}

func install() {
	// 1. 检查kubectl是否安装
	if !checkKubectlInstalled() {
		installKubectl()
	} else {
		fmt.Println("✅ kubectl 已安装")
	}

	// 2. 输入kubeconfig并检查有效性
	kubeconfig := getKubeconfigFromUser()
	if !validateKubeconfig(kubeconfig) {
		fmt.Println("❌ 提供的kubeconfig无效，请检查并重试")
		return
	}

	// 3. 保存kubeconfig到指定目录
	saveKubeconfig(kubeconfig)

	// 4. 安装VSCode扩展
	installVSCodeExtensions()

	// 5. 获取分区信息
	partitions := getPartitions()
	if partitions == nil {
		fmt.Println("❌ 获取分区信息失败，请检查网络连接")
		return
	}

	// 6. 显示分区信息
	listPartitions(partitions)

	fmt.Println("✅ 安装完成")
}

func checkKubectlInstalled() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

func installKubectl() {
	fmt.Println("正在安装kubectl...")

	var cmd *exec.Cmd

	// TODO: Fix if update kubectl version
	version := "v1.32.3"

	switch runtime.GOOS {
	case "darwin": // macOS
		if checkCommandExists("brew") {
			cmd = exec.Command("brew", "install", "kubectl")
		} else {
			fmt.Println("请先安装Homebrew: https://brew.sh/")
			return
		}

	case "linux":
		// 默认下载到/usr/local/bin，询问用户，如果反对，下载到~/.hpcgame/bin，并增加到PATH
		fmt.Println("请问您要将kubectl安装到/usr/local/bin还是~/.hpcgame/bin？")
		fmt.Println("1. /usr/local/bin【默认】")
		fmt.Println("2. ~/.hpcgame/bin")
		fmt.Print("请输入选项 (1/2): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		option := scanner.Text()
		var installPath string
		if option == "2" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Printf("获取用户主目录失败: %s\n", err)
				return
			}
			installPath = filepath.Join(homeDir, kubeconfigDir, "bin")
			err = os.MkdirAll(installPath, 0700)
			if err != nil {
				fmt.Printf("创建目录失败: %s\n", err)
				return
			}
			fmt.Printf("请将%s添加到PATH中\n", installPath)
		} else {
			installPath = "/usr/local/bin"
		}
		cmd = exec.Command("bash", "-c",
			"curl -LO https://dl.k8s.io/release/"+string(version)+"/bin/linux/amd64/kubectl && "+
				"chmod +x kubectl && "+
				"sudo mv kubectl "+installPath)
		if option == "2" {
			fmt.Printf("请将%s添加到PATH中，是否由本程序修改.bashrc与.zshrc？(Y/n): ", installPath)
			scanner.Scan()
			addToPath := scanner.Text()
			// modify PATH for this session to make it work
			os.Setenv("PATH", os.Getenv("PATH")+":"+installPath)
			if addToPath != "Y" && addToPath != "y" && addToPath != "" {
				fmt.Printf("请手动将%s添加到PATH中\n", installPath)
			} else {
				// add to .bash
				bashrcPath := filepath.Join(os.Getenv("HOME"), ".bashrc")
				zshrcPath := filepath.Join(os.Getenv("HOME"), ".zshrc")
				if _, err := os.Stat(bashrcPath); err == nil {
					// bash
					f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						fmt.Printf("打开.bashrc失败: %s\n", err)
						return
					}
					defer f.Close()
					if _, err := f.WriteString(fmt.Sprintf("\nexport PATH=$PATH:%s\n", installPath)); err != nil {
						fmt.Printf("写入.bashrc失败: %s\n", err)
						return
					}
					fmt.Printf("已将%s添加到.bashrc\n", installPath)
				}
				if _, err := os.Stat(zshrcPath); err == nil {
					// zsh, if .zshrc exists
					if _, err := os.Stat(zshrcPath); err == nil {
						f, err := os.OpenFile(zshrcPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
						if err != nil {
							fmt.Printf("打开.zshrc失败: %s\n", err)
							return
						}
						defer f.Close()
						if _, err := f.WriteString(fmt.Sprintf("\nexport PATH=$PATH:%s\n", installPath)); err != nil {
							fmt.Printf("写入.zshrc失败: %s\n", err)
							return
						}
						fmt.Printf("已将%s添加到.zshrc\n", installPath)
					}
				}
			}
		}

	case "windows":
		// 下载kubectl，先尝试winget，然后下载并提示用户放进PATH
		cmd = exec.Command("powershell", "-Command",
			"if (Get-Command winget -ErrorAction SilentlyContinue) { "+
				"winget install --id Kubernetes.kubectl -e } else { "+
				"$url = \"https://dl.k8s.io/release/"+string(version)+"/bin/windows/amd64/kubectl.exe\"; "+
				"$output = 'kubectl.exe'; "+
				"Invoke-WebRequest -Uri $url -OutFile $output; "+
				"Write-Host '请将kubectl.exe移动到PATH中的目录，例如C:\\Windows\\System32'; "+
				"Write-Host '或者手动安装kubectl。教程：https://kubernetes.io/docs/tasks/tools/install-kubectl-windows/'; }")

	default:
		fmt.Printf("不支持的操作系统: %s\n", runtime.GOOS)
		return
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("安装kubectl失败: %s\n%s\n", err, string(output))
		return
	}

	fmt.Println("✅ kubectl安装成功")
}

func listPartitions(partitions []Partition) {
	fmt.Println("可用分区:")
	for cnt, partition := range partitions {
		info := fmt.Sprintf("[%d]分区: %s \n\t简介: %s \n\tCPU限制: %d \n\t内存限制: %dGiB \n",
			cnt, partition.Name, partition.Description, partition.CPULimit, partition.MemoryLimit)
		if partition.GPUTag != "" {
			info += fmt.Sprintf("\n\t可用GPU: %s \n", partition.GPUName)
		}
		info += "\t验证过的镜像列表（也可以使用自定义镜像）: "
		for cmti, image := range partition.Images {
			info += fmt.Sprintf("\n\t\t[%d] %s\n", cmti, image)
		}
		fmt.Println(info)
		fmt.Println("------------------------------------------------")
	}
}

func shellContainer() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	if len(os.Args) < 3 {
		fmt.Println("请指定要连接的容器名称")
		fmt.Println("用法: hpcgame shell <容器名称>")
		return
	}

	containerName := os.Args[2]
	fmt.Printf("正在连接到容器 %s...\n", containerName)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "exec", "-it", containerName, "--", "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("连接到容器失败: %s\n", err)
		return
	}
}

func execInContainer() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	if len(os.Args) < 4 {
		fmt.Println("请指定要执行的容器名称和命令")
		fmt.Println("用法: hpcgame exec <容器名称> <命令>")
		return
	}

	containerName := os.Args[2]
	command := os.Args[3:]

	fmt.Printf("在容器 %s 中执行命令: %s\n", containerName, strings.Join(command, " "))

	kubectlArgs := append([]string{"--kubeconfig", kubeconfigPath, "exec", containerName, "--"}, command...)
	cmd := exec.Command("kubectl", kubectlArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("执行命令失败: %s\n", err)
		return
	}
}

func copyFiles() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	if len(os.Args) < 4 {
		fmt.Println("请指定源文件和目标文件")
		fmt.Println("用法: hpcgame cp <源文件> <目标文件>")
		fmt.Println("例如: hpcgame cp ./local-file.txt my-container:/path/to/file.txt")
		fmt.Println("      hpcgame cp my-container:/path/to/file.txt ./local-file.txt")
		return
	}

	source := os.Args[2]
	destination := os.Args[3]

	// 处理目标为容器但没有指定路径的情况
	if strings.Contains(destination, ":") && strings.HasSuffix(destination, ":") {
		containerName := strings.TrimSuffix(destination, ":")
		destination = containerName + ":~/"
		fmt.Printf("⚠️ 未指定目标路径，将复制到容器 %s 的用户主目录\n", containerName)
	}

	// 处理源为容器但没有指定路径的情况
	if strings.Contains(source, ":") && strings.HasSuffix(source, ":") {
		fmt.Println("❌ 错误: 源文件路径不能为空")
		fmt.Println("用法: hpcgame cp <源文件> <目标文件>")
		return
	}

	fmt.Printf("复制文件: %s -> %s\n", source, destination)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "cp", source, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("复制文件失败: %s\n", err)
		return
	}

	fmt.Println("✅ 文件复制成功")
}

func portForward() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	if len(os.Args) < 4 {
		fmt.Println("请指定容器名称和端口映射")
		fmt.Println("用法: hpcgame portforward <容器名称> <本地端口>:<容器端口>")
		fmt.Println("例如: hpcgame portforward my-container 8080:80")
		return
	}

	containerName := os.Args[2]
	portMapping := os.Args[3]

	fmt.Printf("设置端口转发: %s %s\n", containerName, portMapping)
	fmt.Println("按 Ctrl+C 停止端口转发")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "port-forward", "pod/"+containerName, portMapping)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("设置端口转发失败: %s\n", err)
		return
	}
}

// cached partitions. Update once a day on need
func getPartitions() []Partition {
	// get last update time from ~/.hpcgame/partition_last_update
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户主目录失败: %s\n", err)
		return nil
	}

	// create ~/.hpcgame directory if not exists
	kubeconfigDir := filepath.Join(homeDir, kubeconfigDir)
	if _, err := os.Stat(kubeconfigDir); os.IsNotExist(err) {
		err := os.MkdirAll(kubeconfigDir, 0700)
		if err != nil {
			fmt.Printf("创建目录失败: %s\n", err)
			return nil
		}
		fmt.Printf("创建hpcgame目录: %s\n", kubeconfigDir)
	}

	partitionFile := filepath.Join(kubeconfigDir, "partitions.json")
	lastUpdateFile := filepath.Join(kubeconfigDir, "partition_last_update")
	if _, err := os.Stat(lastUpdateFile); os.IsNotExist(err) {
		// create file
		os.WriteFile(lastUpdateFile, []byte("0"), 0644)
	}
	// read file
	is_outdated := true
	lastUpdateTime := 0
	lastUpdate, err := os.ReadFile(lastUpdateFile)
	if err != nil {
		is_outdated = true
	} else {
		lastUpdateTime, err = strconv.Atoi(string(lastUpdate))
		if err != nil {
			fmt.Printf("解析时间失败: %s\n", err)
			return nil
		}
	}
	// check if last update time is older than 24 hours
	if is_outdated || lastUpdateTime+86400 < int(time.Now().Unix()) {
		// update partitions from https://hpcgame.pku.edu.cn/oss/images/public/partitions.json
		resp, err := http.Get("https://hpcgame.pku.edu.cn/oss/images/public/partitions.json")
		if err != nil {
			fmt.Printf("获取分区信息失败: %s\n", err)
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("获取分区信息失败: %s\n", resp.Status)
			return nil
		}

		// read json, save to ~/.hpcgame/partitions.json, update last update time
		out, err := os.Create(partitionFile)
		if err != nil {
			fmt.Printf("创建文件失败: %s\n", err)
			return nil
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Printf("保存分区信息失败: %s\n", err)
			return nil
		}
		// update last update time
		err = os.WriteFile(lastUpdateFile, []byte(strconv.Itoa(int(time.Now().Unix()))), 0644)
		if err != nil {
			fmt.Printf("更新分区信息失败: %s\n", err)
			return nil
		}

		fmt.Printf("分区信息已更新: %s\n", partitionFile)
	}

	// read partitions from ~/.hpcgame/partitions.json
	data, err := os.ReadFile(partitionFile)
	if err != nil {
		fmt.Printf("读取分区信息失败: %s\n", err)
		return nil
	}
	var partitions []Partition
	err = json.Unmarshal(data, &partitions)
	if err != nil {
		fmt.Printf("解析分区信息失败: %s\n", err)
		return nil
	}
	// print partitions if DEBUG is set
	if os.Getenv("DEBUG") != "" {
		fmt.Printf("分区信息: %s\n", string(data))
		// print partitions
		fmt.Println("可用分区:")
		for _, partition := range partitions {
			fmt.Printf("分区: %s, GPU标签: %s, 镜像: %s\n", partition.Name, partition.GPUTag, strings.Join(partition.Images, ", "))
		}
	}
	return partitions
}

func checkCommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func getKubeconfigFromUser() string {
	fmt.Println("请输入您的kubeconfig内容，可以前往 https://hpcgame.pku.edu.cn/kube/_/ui/#/tokens/ 获取。输入完成后按Ctrl+D（linux、macOS）或Ctrl+Z（windows）结束:")

	var kubeconfig strings.Builder
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		kubeconfig.WriteString(scanner.Text())
		kubeconfig.WriteString("\n")
	}

	return kubeconfig.String()
}

func validateKubeconfig(kubeconfig string) bool {
	// 创建临时文件存储kubeconfig
	tmpFile, err := os.CreateTemp("", "kubeconfig-*")
	if err != nil {
		fmt.Printf("创建临时文件失败: %s\n", err)
		return false
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(kubeconfig); err != nil {
		fmt.Printf("写入临时文件失败: %s\n", err)
		return false
	}
	tmpFile.Close()

	// 使用临时kubeconfig尝试列出节点
	cmd := exec.Command("kubectl", "--kubeconfig", tmpFile.Name(), "get", "nodes")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err != nil {
		fmt.Printf("验证kubeconfig失败: %s\n%s\n", err, stderr.String())
		return false
	}

	fmt.Println("✅ kubeconfig验证成功")
	return true
}

func saveKubeconfig(kubeconfig string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户主目录失败: %s\n", err)
		return
	}

	configDir := filepath.Join(homeDir, kubeconfigDir)
	err = os.MkdirAll(configDir, 0700)
	if err != nil {
		fmt.Printf("创建配置目录失败: %s\n", err)
		return
	}

	kubeconfigPath := filepath.Join(configDir, kubeconfigFile)

	// 检查文件是否存在
	if _, err := os.Stat(kubeconfigPath); err == nil {
		fmt.Printf("kubeconfig文件已存在于 %s\n", kubeconfigPath)
		fmt.Print("是否覆盖? (y/n): ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("读取输入失败: %s\n", err)
			return
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("操作已取消")
			return
		}
	}

	// 写入kubeconfig
	err = os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0600)
	if err != nil {
		fmt.Printf("保存kubeconfig失败: %s\n", err)
		return
	}

	fmt.Printf("✅ kubeconfig已保存到 %s\n", kubeconfigPath)
}

func installVSCodeExtensions() {
	// 检查code命令是否可用
	if _, err := exec.LookPath("code"); err != nil {
		fmt.Println("⚠️ 未找到VSCode命令行工具 'code'")
		fmt.Println("如果您已安装VSCode，请确保'code'命令已添加到PATH中")
		fmt.Println("或者手动安装以下VSCode扩展:")
		fmt.Println("- ms-kubernetes-tools.vscode-kubernetes-tools")
		fmt.Println("- ms-vscode-remote.remote-containers")
		return
	}

	extensions := []string{
		"ms-kubernetes-tools.vscode-kubernetes-tools",
		"ms-vscode-remote.remote-containers",
	}

	fmt.Println("正在安装VSCode扩展...")

	for _, ext := range extensions {
		cmd := exec.Command("code", "--install-extension", ext)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		if err != nil {
			fmt.Printf("安装扩展 %s 失败: %s\n%s\n", ext, err, stderr.String())
		} else {
			fmt.Printf("✅ 安装扩展 %s 成功\n", ext)
		}
	}
}

func getKubeConfig() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户主目录失败: %s\n", err)
		return ""
	}

	kubeconfigPath := filepath.Join(homeDir, kubeconfigDir, kubeconfigFile)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		fmt.Printf("kubeconfig不存在: %s\n请先运行 install 命令\n", kubeconfigPath)
		return ""
	}

	return kubeconfigPath
}

func createContainer() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	// 创建一个新的标志集
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)

	// 定义命令行选项
	partitionFlag := createCmd.String("partition", "", "指定分区名称")
	cpuFlag := createCmd.Int("cpu", 0, "指定CPU核心数")
	memoryFlag := createCmd.Int("memory", 0, "指定内存大小，单位GiB")
	gpuFlag := createCmd.Int("gpu", 0, "指定GPU数量")
	imageFlag := createCmd.String("image", "", "指定容器镜像")
	nameFlag := createCmd.String("name", "", "指定容器名称")
	helpFlag := createCmd.Bool("help", false, "显示帮助信息")

	// 支持短标志
	createCmd.StringVar(partitionFlag, "p", "", "指定分区名称 (简写)")
	createCmd.IntVar(cpuFlag, "c", 0, "指定CPU核心数 (简写)")
	createCmd.IntVar(memoryFlag, "m", 0, "指定内存大小，单位GiB (简写)")
	createCmd.IntVar(gpuFlag, "g", 0, "指定GPU数量 (简写)")
	createCmd.StringVar(imageFlag, "i", "", "指定容器镜像 (简写)")
	createCmd.StringVar(nameFlag, "n", "", "指定容器名称 (简写)")
	createCmd.BoolVar(helpFlag, "h", false, "显示帮助信息 (简写)")

	// 解析命令行参数
	if len(os.Args) < 3 {
		createCmd.Usage()
		return
	}

	err := createCmd.Parse(os.Args[2:])
	if err != nil {
		fmt.Printf("解析参数失败: %s\n", err)
		return
	}

	// 显示帮助
	if *helpFlag {
		fmt.Println("用法: hpcgame create [选项]")
		fmt.Println("选项:")
		createCmd.PrintDefaults()
		return
	}

	// 获取分区信息
	partitions := getPartitions()
	if partitions == nil {
		fmt.Println("获取分区信息失败")
		return
	}

	// 处理分区
	partitionStruct := Partition{}
	partition := *partitionFlag

	// 如果没有提供分区，交互式询问
	if partition == "" {
		listPartitions(partitions)
		fmt.Print("请输入分区名称: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			partition = scanner.Text()
		} else {
			fmt.Println("读取输入失败")
			return
		}
	}

	// 检查分区是否有效
	validPartition := false
	for _, p := range partitions {
		if p.Name == partition {
			validPartition = true
			partitionStruct = p
			break
		}
	}
	if !validPartition {
		fmt.Printf("无效的分区名称: %s\n", partition)
		listPartitions(partitions)
		return
	}

	// 处理CPU
	cpu := *cpuFlag
	if cpu == 0 {
		fmt.Print("请输入CPU核心数: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			cpuValue := scanner.Text()
			parsedCPU, err := strconv.Atoi(cpuValue)
			if err != nil {
				fmt.Printf("无效的CPU值: %s\n", cpuValue)
				return
			}
			cpu = parsedCPU
		} else {
			fmt.Println("读取输入失败")
			return
		}
	}

	// 检查CPU值是否有效
	if cpu <= 0 || cpu > partitionStruct.CPULimit {
		fmt.Printf("无效的CPU值: %d, 分区限制: %d\n", cpu, partitionStruct.CPULimit)
		return
	}

	// 处理内存
	memory := *memoryFlag
	if memory == 0 {
		// 设置默认值
		memory = cpu * 2
		fmt.Printf("未指定内存，使用默认值: %dGiB\n", memory)
	}

	// 检查内存值是否有效
	if memory <= 0 || memory > partitionStruct.MemoryLimit {
		fmt.Printf("无效的内存值: %dGi, 分区限制: %dGi\n", memory, partitionStruct.MemoryLimit)
		return
	}

	// 处理GPU
	gpu := *gpuFlag

	// 处理镜像
	image := *imageFlag
	if image == "" {
		if len(partitionStruct.Images) > 0 {
			image = partitionStruct.Images[0]
			fmt.Printf("未指定镜像，使用默认值: %s\n", image)
		} else {
			fmt.Println("分区没有可用镜像，请手动指定镜像")
			fmt.Print("请输入镜像名称: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				image = scanner.Text()
			} else {
				fmt.Println("读取输入失败")
				return
			}
		}
	}

	// 处理容器名称
	name := *nameFlag
	if name == "" {
		name = fmt.Sprintf("container-%d", os.Getpid())
	}

	// 创建容器
	fmt.Printf("正在创建容器 %s...\n", name)
	createErr := deployContainer(kubeconfigPath, partitionStruct, name, cpu, memory, gpu, image)
	if createErr != nil {
		fmt.Printf("创建容器失败: %s\n", createErr)
		return
	}

	fmt.Printf("✅ 容器 %s 创建请求已发起\n", name)
}

func deployContainer(kubeconfigPath string, partition Partition, name string, cpu int, memory int, gpu int, image string) error {
	// 生成YAML配置
	yamlConfig := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
spec:
  nodeSelector:
    hpc.lcpu.dev/partition: %s
  containers:
  - name: container
    securityContext:
      capabilities:
        add: ["SYS_PTRACE", "IPC_LOCK"]
    image: %s
    command: ["sleep", "infinity"]
    resources:
      requests:
        cpu: %dm
        memory: %dGi
`, name, partition.Name, image, cpu*1000, memory)
	// 如果有GPU
	if gpu > 0 {
		gpulimit := fmt.Sprintf("%s: %d", partition.GPUTag, gpu)
		yamlConfig += fmt.Sprintf(`        %s
      limits:
        cpu: %dm
        memory: %dGi
        %s
`, gpulimit, cpu*1000, memory, gpulimit)
	}

	// if debug mode, print yamlConfig
	if os.Getenv("DEBUG") != "" {
		fmt.Printf("生成的YAML配置:\n%s\n", yamlConfig)
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "pod-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %s", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlConfig); err != nil {
		return fmt.Errorf("写入临时文件失败: %s", err)
	}
	tmpFile.Close()

	// 应用配置
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tmpFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err != nil {
		return fmt.Errorf("部署容器失败: %s\n%s", err, stderr.String())
	}

	return nil
}

func listContainers() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	fmt.Println("正在获取容器列表...")

	// 获取当前命名空间
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "config", "view", "--minify", "-o", "jsonpath={..namespace}")
	var namespaceOutput bytes.Buffer
	cmd.Stdout = &namespaceOutput
	err := cmd.Run()

	namespace := namespaceOutput.String()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	// 列出当前命名空间中的所有Pod
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "get", "pods", "-n", namespace, "-o", "wide")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("获取容器列表失败: %s\n", err)
		return
	}
}

func deleteContainer() {
	kubeconfigPath := getKubeConfig()
	if kubeconfigPath == "" {
		return
	}

	if len(os.Args) < 3 {
		fmt.Println("请指定要删除的容器名称")
		fmt.Println("用法: hpcgame delete <容器名称>")
		return
	}

	containerName := os.Args[2]
	fmt.Printf("正在删除容器 %s，请等待...\n", containerName)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "delete", "pod", containerName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("删除容器失败: %s\n", err)
		return
	}

	fmt.Printf("✅ 容器 %s 已删除\n", containerName)
}
