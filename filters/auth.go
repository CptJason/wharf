package filters

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/context"

	"github.com/dockercn/wharf/models"
	"github.com/dockercn/wharf/modules"
	"github.com/dockercn/wharf/utils"
)

const (
	PERMISSION_WRITE = iota
	PERMISSION_READ
)

func FilterAuth(ctx *context.Context) {
	var namespace, repository string
	var permission int

	auth := true
	user := new(models.User)

	if strings.Index(ctx.Request.URL.String(), "_ping") > 0 {
		goto AUTH
	}

	if strings.Index(ctx.Request.URL.String(), "v1") > 0 {
		namespace = strings.Split(string(ctx.Input.Params[":splat"]), "/")[1]
		repository = strings.Split(string(ctx.Input.Params[":splat"]), "/")[2]
	}

	if strings.Index(ctx.Request.URL.String(), "v2") > 0 {
		namespace = strings.Split(string(ctx.Input.Params[":splat"]), "/")[0]
		repository = strings.Split(string(ctx.Input.Params[":splat"]), "/")[1]
	}

	//Get Permission
	permission = getPermission(ctx.Input.Method())

	//Check Authorization In Header
	if len(ctx.Input.Header("Authorization")) == 0 || strings.Index(ctx.Input.Header("Authorization"), "Basic") == -1 {
		beego.Trace("[Docker Registry API] Header Authorization Error!")
		auth = false
		goto AUTH
	}

	//Check Username, Password And Get User
	if username, passwd, err := utils.DecodeBasicAuth(ctx.Input.Header("Authorization")); err != nil {
		beego.Trace("[Docker Registry API] DecodeBasicAuth Error!")
		auth = false
		goto AUTH
	} else {
		if err := user.Get(username, passwd); err != nil {
			beego.Trace("[Docker Registry API] Username And Password Check Error:", err.Error())
			auth = false
			goto AUTH
		}
	}

	//Docker Registry V1 Image Don't Check User/Org Permission
	if isImageResource(ctx.Request.URL.String()) == true {
		beego.Trace("[Docker Registry API] Docker Registry V1 Image Don't Check User/Org Permission!")
		goto AUTH
	}

	beego.Trace("[Docker Registry API] User:", user)

	//Check Org Permission
	if user.Username != namespace {
		beego.Trace("[Docker Registry API] Check Org Privilege")
		auth = checkOrgRepositoryPermission(user, namespace, repository, permission)
	}

AUTH:
	beego.Debug("[Docker Registry API] Authorization Result:", auth)

	if auth == false {
		result := map[string][]modules.ErrorDescriptor{"errors": []modules.ErrorDescriptor{modules.ErrorDescriptors[modules.APIErrorCodeUnauthorized]}}

		data, _ := json.Marshal(result)

		ctx.Output.Context.Output.SetStatus(http.StatusNotFound)
		ctx.Output.Context.Output.Body(data)
		return
	}
}

func getPermission(method string) int {
	write := map[string]string{"POST": "POST", "PUT": "PUT", "DELETE": "DELETE"}
	read := map[string]string{"HEAD": "HEAD", "GET": "GET"}

	if _, ok := write[method]; ok == true {
		return PERMISSION_WRITE
	}

	if _, ok := read[method]; ok == true {
		return PERMISSION_READ
	}

	return PERMISSION_READ
}

func isImageResource(url string) bool {
	if strings.Index(url, "images") == -1 {
		return false
	}

	if strings.Index(url, "images") > 0 {
		return true
	}

	return false
}

func checkOrgRepositoryPermission(user *models.User, namespace, repository string, permission int) bool {
	owner := false

	//Check Org exists
	org := new(models.Organization)
	if has, _, _ := org.Has(namespace); has == false {
		return false
	}

	//Check Owner, don't care Join team
	for _, k := range user.Organizations {
		if org.UUID == k {
			owner = true
		}
	}

	//Check Repository
	repo := new(models.Repository)
	if has, _, _ := repo.Has(namespace, repository); has == false {
		if owner == true {
			return true
		} else {
			return false
		}
	}

	//Loop Team
	for _, k := range user.Teams {
		team := new(models.Team)

		if err := team.Get(k); err != nil {
			return false
		}

		//Loop Team Privileges
		for _, v := range team.TeamPrivileges {
			p := new(models.Privilege)
			if err := p.Get(v); err != nil {
				return false
			}

			//Got User Team Privilege
			if p.Repository == repo.UUID {
				if p.Privilege == true && permission == PERMISSION_WRITE {
					return true
				} else if p.Privilege == false && permission == PERMISSION_READ {
					return true
				} else {
					return false
				}
			}

		}
	}

	return false
}