package ns1

import (
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"

	ns1 "gopkg.in/ns1/ns1-go.v2/rest"
	"gopkg.in/ns1/ns1-go.v2/rest/model/account"
)

func userResource() *schema.Resource {
	s := map[string]*schema.Schema{
		"name": {
			Type:     schema.TypeString,
			Required: true,
		},
		"username": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		"email": {
			Type:     schema.TypeString,
			Required: true,
		},
		"notify": {
			Type:     schema.TypeMap,
			Optional: true,
			Elem:     schema.TypeBool,
		},
		"teams": {
			Type:     schema.TypeList,
			Optional: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
	}
	s = addPermsSchema(s)
	return &schema.Resource{
		Schema: s,
		Create: UserCreate,
		Read:   UserRead,
		Update: UserUpdate,
		Delete: UserDelete,
	}
}

func userToResourceData(d *schema.ResourceData, u *account.User) error {
	d.SetId(u.Username)
	d.Set("name", u.Name)
	d.Set("email", u.Email)
	d.Set("teams", u.TeamIDs)
	notify := make(map[string]bool)
	notify["billing"] = u.Notify.Billing
	d.Set("notify", notify)
	permissionsToResourceData(d, u.Permissions)
	return nil
}

func resourceDataToUser(u *account.User, d *schema.ResourceData) error {
	u.Name = d.Get("name").(string)
	u.Username = d.Get("username").(string)
	u.Email = d.Get("email").(string)
	if v, ok := d.GetOk("teams"); ok {
		teamsRaw := v.([]interface{})
		u.TeamIDs = make([]string, len(teamsRaw))
		for i, team := range teamsRaw {
			u.TeamIDs[i] = team.(string)
		}
	} else {
		u.TeamIDs = make([]string, 0)
	}
	if v, ok := d.GetOk("notify"); ok {
		notifyRaw := v.(map[string]interface{})
		u.Notify.Billing = notifyRaw["billing"].(bool)
	}
	u.Permissions = resourceDataToPermissions(d)
	return nil
}

// UserCreate creates the given user in ns1
func UserCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ns1.Client)
	u := account.User{}
	if err := resourceDataToUser(&u, d); err != nil {
		return err
	}
	if _, err := client.Users.Create(&u); err != nil {
		return err
	}

	// If a user is assigned to at least one team, then it's permissions need to be refreshed
	// because the current user permissions in Terraform state will be out of date.
	if len(u.TeamIDs) > 0 {
		updatedUser, _, err := client.Users.Get(u.Username)
		if err != nil {
			return err
		}

		return userToResourceData(d, updatedUser)
	}

	return userToResourceData(d, &u)
}

// UserRead reads the given users data from ns1
func UserRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ns1.Client)
	u, _, err := client.Users.Get(d.Id())
	if err != nil {
		// No custom error type is currently defined in the SDK for a non-existent user.
		if strings.Contains(err.Error(), "User not found") {
			log.Printf("[DEBUG] NS1 user (%s) not found", d.Id())
			d.SetId("")
			return nil
		}

		return err
	}
	return userToResourceData(d, u)
}

// UserDelete deletes the given user from ns1
func UserDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ns1.Client)
	_, err := client.Users.Delete(d.Id())
	d.SetId("")
	return err
}

// UserUpdate updates the user with given parameters in ns1
func UserUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ns1.Client)
	u := account.User{
		Username: d.Id(),
	}
	if err := resourceDataToUser(&u, d); err != nil {
		return err
	}

	if _, err := client.Users.Update(&u); err != nil {
		return err
	}

	// If a user's teams has changed then the permissions on the user need to be refreshed
	// because the current user permissions in Terraform state will be out of date.
	if d.HasChange("teams") {
		updatedUser, _, err := client.Users.Get(d.Id())
		if err != nil {
			return err
		}

		return userToResourceData(d, updatedUser)
	}

	return userToResourceData(d, &u)
}
