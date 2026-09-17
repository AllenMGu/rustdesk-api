package config

type Gin struct {
	ApiAddr       string `mapstructure:"api-addr"`
	AdminAddr     string `mapstructure:"admin-addr"`
	Mode          string
	ResourcesPath string `mapstructure:"resources-path"`
	TrustProxy    string `mapstructure:"trust-proxy"`
	// CorsOrigins 跨域请求允许的来源白名单（Origin 精确匹配，如 https://console.example.com）。
	// 为空时不向任何来源下发跨域响应头（不再反射任意 Origin）。
	CorsOrigins []string `mapstructure:"cors-origins"`
}
