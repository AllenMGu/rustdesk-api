package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Ldap struct{}

// Config returns the effective LDAP configuration with secrets redacted and
// indicates fields that are locked by environment variables.
func (ct *Ldap) Config(c *gin.Context) {
	response.Success(c, service.AllService.LdapService.SettingsDocument())
}

// Update validates, encrypts, persists, and atomically applies LDAP settings.
func (ct *Ldap) Update(c *gin.Context) {
	request := service.LdapSaveRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, 101, "Invalid LDAP settings: "+err.Error())
		return
	}
	document, err := service.AllService.LdapService.SaveSettings(request)
	if err != nil {
		response.Fail(c, 101, "Unable to save LDAP settings: "+err.Error())
		return
	}
	response.Success(c, document)
}

// Test checks the submitted form without persisting it. Test credentials are
// used only for the LDAP bind and are never returned or logged.
func (ct *Ldap) Test(c *gin.Context) {
	request := service.LdapTestRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, 101, "Invalid LDAP test request: "+err.Error())
		return
	}
	result := service.AllService.LdapService.TestConfiguration(request)
	response.Success(c, result)
}

func (ct *Ldap) Rollback(c *gin.Context) {
	document, err := service.AllService.LdapService.RollbackSettings()
	if err != nil {
		response.Fail(c, 101, "Unable to roll back LDAP settings: "+err.Error())
		return
	}
	response.Success(c, document)
}
