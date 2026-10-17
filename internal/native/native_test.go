package native

import "testing"

func TestContainerEnvironmentDisablesNativeControl(t *testing.T) {
	t.Setenv("container", "docker")
	if Available() {
		t.Fatal("容器标记未关闭原生功能")
	}
}
func TestDockerBuildCannotBeEnabledByEnvironment(t *testing.T) {
	if !dockerBuild {
		t.Skip("仅 Docker 构建检查")
	}
	t.Setenv("container", "")
	t.Setenv("PORTER_CONTROL_SOCKET", "/any/control.sock")
	t.Setenv("PORTER_NATIVE", "true")
	if Available() {
		t.Fatal("环境变量绕过 Docker 构建边界")
	}
}
