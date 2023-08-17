# kc_delete_older_than
Keycloak Delete Users Older Than Specified Date

A simple tool to specify in keycloak the realm, and the age of users to delete.

If you want to bulk delete users who were created more than 30 days ago, you would run :

`kc_delete_older_than --days 30`

## Usage

```bash
Usage of ./kc_delete_older_than:
  -b, --channelBuffer int              the number of buffered spaces in the channel buffer (default 10000)
  -u, --clientId string                The API user that will execute the calls. (default "admin")
  -s, --clientRealm clientId           The realm in which the clientId exists (default "master")
  -p, --clientSecret clientId          The secret for the keycloak user defined by clientId (default "admin")
      --days int                       the number of days, after which users are deleted (default -1)
      --deleteDate string              The date after which users will be deleted. Format: YYYY-MM-DD
  -d, --destinationRealm clientRealm   The realm in keycloak where the users are to be created. This may or may not be the same as the clientRealm (default "delete")
      --dryRun                         if true, then no users will be deleted, it will just log the outcome.
      --listOnly                       if true, then it will only generate a list the users that will be deleted.
      --logCmdValues                   if true, then the command line values will be logged.
      --logDir string                  The logging directory. (default "/tmp")
  -z, --loginAsAdmin                   if true, then it will login as admin user, rather than a client.
      --searchMax int                  The maximum number of users to search through. (default 1000)
      --searchMin int                  The starting number of users to search through.
  -t, --threads int                    the number of threads to run the keycloak import (default 10)
  -w, --url string                     The URL of the keycloak server. (default "http://127.0.0.1:8080")
      --useLegacyKeycloak              if true, then it will use the legacy keycloak client url.
  -v, --validateLoginOnly              if true, then it will only validate the login.
      --version                        if true, Then it will show the version.
pflag: help requested
```