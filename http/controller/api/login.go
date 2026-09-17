package api

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	apiResp "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
	"strconv"
	"time"
)

type Login struct {
}

// Login 登录
// @Tags 登录
// @Summary 登录
// @Description 登录
// @Accept  json
// @Produce  json
// @Param body body api.LoginForm true "登录表单"
// @Success 200 {object} apiResp.LoginRes
// @Failure 500 {object} response.ErrorResponse
// @Router /login [post]
func (l *Login) Login(c *gin.Context) {
	if global.Config.App.DisablePwdLogin {
		response.Error(c, response.TranslateMsg(c, "PwdLoginDisabled"))
		return
	}

	// 检查登录限制
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()

	// IP 级封禁检查（若配置启用）：直接以 429 拒绝
	if banned, _ := loginLimiter.CheckSecurityStatus(clientIp); banned {
		global.Logger.Warn(fmt.Sprintf("Login blocked (ip banned): ip=%s", clientIp))
		l.tooMany(c, response.TranslateMsg(c, "Banned"), retryAfter(loginLimiter.BanUntil(clientIp)))
		return
	}

	f := &api.LoginForm{}
	err := c.ShouldBindJSON(f)
	//fmt.Println(f)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, errList[0])
		return
	}

	// 账号级 (IP, 用户名) 临时阻断检查：桌面客户端不引入验证码，
	// 用 429 + Retry-After 做短时退避
	if blocked, blockedUntil := loginLimiter.CheckAccountBlock(clientIp, f.Username); blocked {
		global.Logger.Warn(fmt.Sprintf("Login blocked (account throttle): user=%s ip=%s until=%s", f.Username, clientIp, blockedUntil.Format(time.RFC3339)))
		l.tooMany(c, response.TranslateMsg(c, "TooManyRequests"), retryAfter(blockedUntil))
		return
	}

	u := service.AllService.UserService.InfoByUsernamePassword(f.Username, f.Password)

	if u.Id == 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		loginLimiter.RecordAccountFailure(clientIp, f.Username)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "UsernameOrPasswordError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, response.TranslateMsg(c, "UsernameOrPasswordError"))
		return
	}

	if !service.AllService.UserService.CheckUserEnable(u) {
		// 禁用账号同样计入失败，防止爆破者借禁用账号持续探测
		loginLimiter.RecordFailedAttempt(clientIp)
		loginLimiter.RecordAccountFailure(clientIp, f.Username)
		response.Error(c, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	//登录成功，清除该 IP 与该账号组合的失败记录
	loginLimiter.RemoveAttempts(clientIp)
	loginLimiter.ClearAccountFailures(clientIp, f.Username)

	//根据refer判断是webclient还是app
	ref := c.GetHeader("referer")
	if ref != "" {
		f.DeviceInfo.Type = model.LoginLogClientWeb
	}

	ut, err := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   f.DeviceInfo.Type,
		DeviceId: f.Id,
		Uuid:     f.Uuid,
		Ip:       c.ClientIP(),
		Type:     model.LoginLogTypeAccount,
		Platform: f.DeviceInfo.Os,
	})
	if err != nil {
		// token 生成失败：登录失败关闭，不落 token/登录日志，返回通用错误
		global.Logger.Errorf("login rejected: token generation failed: %v", err)
		response.Error(c, response.TranslateMsg(c, "OperationFailed"))
		return
	}

	c.JSON(http.StatusOK, apiResp.LoginRes{
		AccessToken: ut.Token,
		Type:        "access_token",
		User:        *(&apiResp.UserPayload{}).FromUser(u),
	})
}

// tooMany 返回 429 Too Many Requests 并携带 Retry-After
func (l *Login) tooMany(c *gin.Context, message string, retryAfterSec int) {
	if retryAfterSec > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfterSec))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, response.ErrorResponse{Error: message})
}

func retryAfter(until time.Time) int {
	sec := int(time.Until(until).Seconds()) + 1
	if sec < 0 {
		sec = 0
	}
	return sec
}

// LoginOptions
// @Tags 登录
// @Summary 登录选项
// @Description 登录选项
// @Accept  json
// @Produce  json
// @Success 200 {object} []string
// @Failure 500 {object} response.ErrorResponse
// @Router /login-options [get]
func (l *Login) LoginOptions(c *gin.Context) {
	ops := service.AllService.OauthService.GetOauthProviders()
	if global.Config.App.WebSso {
		ops = append(ops, model.OauthTypeWebauth)
	}
	var oidcItems []map[string]string
	for _, v := range ops {
		oidcItems = append(oidcItems, map[string]string{"name": v})
	}
	common, err := json.Marshal(oidcItems)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "SystemError")+err.Error())
		return
	}
	var res []string
	res = append(res, "common-oidc/"+string(common))
	for _, v := range ops {
		res = append(res, "oidc/"+v)
	}
	c.JSON(http.StatusOK, res)
}

// Logout
// @Tags 登录
// @Summary 登出
// @Description 登出
// @Accept  json
// @Produce  json
// @Success 200 {string} string
// @Failure 500 {object} response.ErrorResponse
// @Router /logout [post]
func (l *Login) Logout(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	token, ok := c.Get("token")
	if ok {
		service.AllService.UserService.Logout(u, token.(string))
	}
	c.JSON(http.StatusOK, nil)

}
