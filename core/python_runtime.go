package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/smallfawn/sillyGirl/utils"
)

const pythonRequiredVersion = "3.12"

var pythonSillygirlModuleCache sync.Map
var pythonPathEnvCache sync.Map
var pythonCommandCache = struct {
	sync.Mutex
	bin  string
	args []string
}{}

func ensurePythonSillygirlModule() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("SILLYGIRL_PYTHON_PATH")); configured != "" {
		if _, ok := pythonSillygirlModuleCache.Load(configured); ok {
			return configured, nil
		}
		if err := validatePythonRuntimePath(configured); err != nil {
			return "", err
		}
		pythonSillygirlModuleCache.Store(configured, true)
		return configured, nil
	}
	dir := filepath.Join(utils.ExecPath, "language", "python")
	if _, ok := pythonSillygirlModuleCache.Load(dir); ok {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	for _, name := range []string{"sillygirl.py", "srpc_pb2.py", "srpc_pb2_grpc.py"} {
		if err := copyPythonRuntimeFile(name, filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	pythonSillygirlModuleCache.Store(dir, true)
	return dir, nil
}

func validatePythonRuntimePath(dir string) error {
	for _, name := range []string{"sillygirl.py", "srpc_pb2.py", "srpc_pb2_grpc.py"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return fmt.Errorf("Python sillygirl 运行时文件不存在：%s", path)
		}
	}
	return nil
}

func copyPythonRuntimeFile(name, target string) error {
	for _, source := range pythonRuntimeSourceCandidates(name) {
		if err := copyFile(source, target); err == nil {
			return nil
		}
	}
	return fmt.Errorf("缺少 Python sillygirl 运行时文件：%s", name)
}

func pythonRuntimeSourceCandidates(name string) []string {
	wd, _ := os.Getwd()
	candidates := []string{
		filepath.Join("proto3", name),
		filepath.Join("..", "proto3", name),
		filepath.Join(utils.ExecPath, "proto3", name),
		filepath.Join(filepath.Dir(utils.ExecPath), "proto3", name),
		filepath.Join("/app", "proto3", name),
	}
	if wd != "" {
		candidates = append(candidates, filepath.Join(wd, "proto3", name))
	}
	return candidates
}

func resolvePythonCommand() (string, []string, error) {
	pythonCommandCache.Lock()
	if pythonCommandCache.bin != "" {
		bin := pythonCommandCache.bin
		args := append([]string{}, pythonCommandCache.args...)
		pythonCommandCache.Unlock()
		return bin, args, nil
	}
	pythonCommandCache.Unlock()

	if configured := strings.TrimSpace(os.Getenv("SILLYGIRL_PYTHON_BIN")); configured != "" {
		bin, args := splitCommand(configured)
		if bin == "" {
			return "", nil, errors.New("SILLYGIRL_PYTHON_BIN 为空")
		}
		if pythonCommandVersion(bin, args) == pythonRequiredVersion {
			cachePythonCommand(bin, args)
			return bin, args, nil
		}
		return "", nil, fmt.Errorf("SILLYGIRL_PYTHON_BIN 必须指向 Python %s", pythonRequiredVersion)
	}

	candidates := [][]string{
		{"python3.12"},
		{"python3"},
		{"python"},
		{"py", "-3.12"},
	}
	for _, candidate := range candidates {
		bin := candidate[0]
		args := candidate[1:]
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		if pythonCommandVersion(bin, args) == pythonRequiredVersion {
			cachePythonCommand(bin, args)
			return bin, args, nil
		}
	}
	return "", nil, errors.New("未找到 Python 3.12，请安装 Python 3.12 或使用 Docker 镜像内置运行时")
}

func cachePythonCommand(bin string, args []string) {
	pythonCommandCache.Lock()
	pythonCommandCache.bin = bin
	pythonCommandCache.args = append([]string{}, args...)
	pythonCommandCache.Unlock()
}

func splitCommand(command string) (string, []string) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

func pythonCommandVersion(bin string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	versionArgs := append(append([]string{}, args...), "-c", "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
	output, err := exec.CommandContext(ctx, bin, versionArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func pythonPluginPathEnv(runtimePath string) string {
	cacheKey := runtimePath + string(os.PathListSeparator) + os.Getenv("PYTHONPATH")
	if value, ok := pythonPathEnvCache.Load(cacheKey); ok {
		return value.(string)
	}
	items := []string{}
	candidates := append([]string{runtimePath}, pythonPipxSitePackageDirs()...)
	candidates = append(candidates, pythonPackagesDir(), os.Getenv("PYTHONPATH"))
	for _, item := range candidates {
		for _, path := range filepath.SplitList(item) {
			path = strings.TrimSpace(path)
			if path != "" {
				items = append(items, path)
			}
		}
	}
	seen := map[string]bool{}
	paths := []string{}
	for _, item := range items {
		key := strings.ToLower(filepath.Clean(item))
		if seen[key] {
			continue
		}
		seen[key] = true
		paths = append(paths, item)
	}
	value := strings.Join(paths, string(os.PathListSeparator))
	pythonPathEnvCache.Store(cacheKey, value)
	return value
}

func registerPythonPluginConfigSchema(path, uuid string) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(uuid) == "" {
		return errors.New("插件路径或 UUID 为空")
	}
	bin, args, err := resolvePythonCommand()
	if err != nil {
		return err
	}
	pythonPath, err := ensurePythonSillygirlModule()
	if err != nil {
		return err
	}
	if err := ensurePipxRuntimeEnv(); err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "sillygirl-python-plugin-schema-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	temp.Close()
	defer os.Remove(tempPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmdArgs := append(append([]string{}, args...), "-u", path)
	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Dir = nodePluginWorkDir(path)
	cmd.Env = append(os.Environ(),
		"PYTHONPATH="+pythonPluginPathEnv(pythonPath),
		"PYTHONDONTWRITEBYTECODE=1",
		"PYTHONUNBUFFERED=1",
		"PLUGIN_ID="+uuid,
		"PLUGIN_CONFIG_JSON="+string(utils.JsonMarshal(getPluginUserConfig(uuid))),
		"SILLYGIRL_CONFIG_REGISTER_ONLY=true",
		"SILLYGIRL_CONFIG_SCHEMA_FILE="+tempPath,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("配置注册超时：%v", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("配置注册脚本执行失败：%v：%s", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(tempPath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return errors.New("插件没有导出配置 schema")
	}
	schema := map[string]interface{}{}
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("配置 schema 解析失败：%v", err)
	}
	if len(schema) == 0 {
		return errors.New("配置 schema 为空")
	}
	if _, _, err := SetBucketKeyValue(pluginConfigSchemas, uuid, schema); err != nil {
		return err
	}
	console.Log("已注册 Python 插件配置 %s (%s)", filepath.Base(path), uuid)
	return nil
}
