package commands

import (
	"errors"
	"fmt"
	"github.com/pivotal-cf/jhanda"
	"github.com/pivotal-cf/om/api"
)

type ConfigureLDAPAuthentication struct {
	service configureAuthenticationService
	logger  logger
	Options struct {
		ConfigFile                string `long:"config"                short:"c"  description:"path to yml file for configuration (keys must match the following command line flags)"`
		DecryptionPassphrase      string `long:"decryption-passphrase" short:"dp" required:"true" description:"passphrase used to encrypt the installation"`
		HTTPProxyURL              string `long:"http-proxy-url"                                   description:"proxy for outbound HTTP network traffic"`
		HTTPSProxyURL             string `long:"https-proxy-url"                                  description:"proxy for outbound HTTPS network traffic"`
		NoProxy                   string `long:"no-proxy"                                         description:"comma-separated list of hosts that do not go through the proxy"`
		EmailAttribute            string `long:"email-attribute"                  required:"true" description:"name of the LDAP attribute that contains the users email address"`
		GroupSearchBase           string `long:"group-search-base"                required:"true" description:"start point for a user group membership search, and sequential nested searches"`
		GroupSearchFilter         string `long:"group-search-filter"              required:"true" description:"search filter to find the groups to which a user belongs, e.g. 'member={0}'"`
		LDAPPassword              string `long:"ldap-password"                    required:"true" description:"password for ldap-username DN"`
		LDAPRBACAdminGroup        string `long:"ldap-rbac-admin-group-name"       required:"true" description:"the name of LDAP group whose members should be considered admins of OpsManager"`
		LDAPReferral              string `long:"ldap-referrals"                   required:"true" description:"configure the UAA LDAP referral behavior"`
		LDAPUsername              string `long:"ldap-username"                    required:"true" description:"DN for the LDAP credentials used to search the directory"`
		ServerSSLCert             string `long:"server-ssl-cert"                                  description:"the server certificate when using ldaps://"`
		ServerURL                 string `long:"server-url"                       required:"true" description:"URL to the ldap server, must start with ldap:// or ldaps://"`
		UserSearchBase            string `long:"user-search-base"                 required:"true" description:"a base at which the search starts, e.g. 'ou=users,dc=mycompany,dc=com'"`
		UserSearchFilter          string `long:"user-search-filter"               required:"true" description:"search filter used for the query. Takes one parameter, user ID defined as {0}. e.g. 'cn={0}'"`
		SkipCreateBoshAdminClient bool   `long:"skip-create-bosh-admin-client"         description:"create a UAA client on the Bosh Director, whose credentials can be passed to the BOSH CLI to execute BOSH commands. Default is false."`
	}
}

func NewConfigureLDAPAuthentication(service configureAuthenticationService, logger logger) ConfigureLDAPAuthentication {
	return ConfigureLDAPAuthentication{
		service: service,
		logger:  logger,
	}
}

func (ca ConfigureLDAPAuthentication) Execute(args []string) error {
	err := loadConfigFile(args, &ca.Options, nil)
	if err != nil {
		return fmt.Errorf("could not parse configure-ldap-authentication flags: %s", err)
	}

	ensureAvailabilityOutput, err := ca.service.EnsureAvailability(api.EnsureAvailabilityInput{})
	if err != nil {
		return fmt.Errorf("could not determine initial configuration status: %s", err)
	}

	if ensureAvailabilityOutput.Status == api.EnsureAvailabilityStatusUnknown {
		return errors.New("could not determine initial configuration status: received unexpected status")
	}

	if ensureAvailabilityOutput.Status != api.EnsureAvailabilityStatusUnstarted {
		ca.logger.Printf("configuration previously completed, skipping configuration")
		return nil
	}

	ca.logger.Printf("configuring LDAP authentication...")

	input := api.SetupInput{
		IdentityProvider:                 "ldap",
		EULAAccepted:                     "true",
		DecryptionPassphrase:             ca.Options.DecryptionPassphrase,
		DecryptionPassphraseConfirmation: ca.Options.DecryptionPassphrase,
		HTTPProxyURL:                     ca.Options.HTTPProxyURL,
		HTTPSProxyURL:                    ca.Options.HTTPSProxyURL,
		NoProxy:                          ca.Options.NoProxy,
		LDAPSettings: &api.LDAPSettings{
			EmailAttribute:     ca.Options.EmailAttribute,
			GroupSearchBase:    ca.Options.GroupSearchBase,
			GroupSearchFilter:  ca.Options.GroupSearchFilter,
			LDAPPassword:       ca.Options.LDAPPassword,
			LDAPRBACAdminGroup: ca.Options.LDAPRBACAdminGroup,
			LDAPReferral:       ca.Options.LDAPReferral,
			LDAPUsername:       ca.Options.LDAPUsername,
			ServerSSLCert:      ca.Options.ServerSSLCert,
			ServerURL:          ca.Options.ServerURL,
			UserSearchBase:     ca.Options.UserSearchBase,
			UserSearchFilter:   ca.Options.UserSearchFilter,
		},
	}

	info, err := ca.service.Info()
	if err != nil {
		return err
	}

	versionAtLeast24, err := info.VersionAtLeast(2, 4)
	if err != nil {
		return err
	}

	if versionAtLeast24 {
		input.CreateBoshAdminClient = boolStringFromType(!ca.Options.SkipCreateBoshAdminClient)
	}

	_, err = ca.service.Setup(input)
	if err != nil {
		return fmt.Errorf("could not configure authentication: %s", err)
	}

	ca.logger.Printf("waiting for configuration to complete...")
	for ensureAvailabilityOutput.Status != api.EnsureAvailabilityStatusComplete {
		ensureAvailabilityOutput, err = ca.service.EnsureAvailability(api.EnsureAvailabilityInput{})
		if err != nil {
			return fmt.Errorf("could not determine final configuration status: %s", err)
		}
	}

	ca.logger.Printf("configuration complete")

	if ca.Options.SkipCreateBoshAdminClient {
		ca.logger.Printf(`
Note: BOSH admin client NOT automatically created.
This was skipped due to the 'skip-create-bosh-admin-client' flag.
`)
		return nil
	}

	if !versionAtLeast24 {
		ca.logger.Printf(`
Note: BOSH admin client NOT automatically created.
This is only supported in OpsManager 2.4 and up.
`)
		return nil
	}

	ca.logger.Printf(`
BOSH admin client will be created when the director is deployed.
The client secret can then be found in the Ops Manager UI:
director tile -> Credentials tab -> click on 'Link to Credential' for 'Uaa Bosh Client Credentials'
Note both the client ID and secret.
`)

	return nil
}

func (ca ConfigureLDAPAuthentication) Usage() jhanda.Usage {
	return jhanda.Usage{
		Description:      "This unauthenticated command helps setup the authentication mechanism for your Ops Manager with LDAP.",
		ShortDescription: "configures Ops Manager with LDAP authentication",
		Flags:            ca.Options,
	}
}
