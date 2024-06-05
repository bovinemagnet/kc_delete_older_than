# README #

This is a simple tool to delete users in a Keycloak realm that are older than a certain number of days or date.

> **_NOTE:_**  `--days=0` includes today.

> **_NOTE:_** `--deleteDate=YYYY-MM-DD` will delete all users that are on that date or older than the date specified.

> **_WARNING:_**  This tool is a blunt object, and uses UTC internally, so dates and days may be off by 11hrs or so.


## Getting Help ##

Passing `-h` or `--help` to the command will display the help for the command.

## kc_delete_older_than ##

If you want to bulk delete users who were created more than 30 days ago, you would run :

`kc_delete_older_than --days 30`

## Usage ##

```bash
Usage of ./kc_delete_older_than:
-b, --channelBuffer int                             the number of buffered spaces in the channel buffer (default 10000)
-u, --clientId string                               The API user that will execute the calls. (default "admin")
-s, --clientRealm clientId                          The realm in which the clientId exists (default "master")
-p, --clientSecret clientId                         The secret for the keycloak user defined by clientId (default "admin")
--countTotalUsersOnly GET /{realm}/user/count   if true, then just  do a call to GET /{realm}/user/count.
--days int                                      the number of days, after which users are deleted (default -1)
--deleteDate string                             The date after which users will be deleted. Format: YYYY-MM-DD
-d, --destinationRealm clientRealm                  The realm in keycloak where the users are to be created. This may or may not be the same as the clientRealm(default "delete")
--dryRun                                        if true, then no users will be deleted, it will just log the outcome.
--headerKey string                              The header key to use for the login.
--headerValue string                            The header value to use for the login.
--listOnly                                      if true, then it will only generate a list the users that will be deleted.
--logCmdValues                                  if true, then the command line values will be logged.
--logDir string                                 The logging directory. (default "/tmp")
-z, --loginAsAdmin                                  if true, then it will login as admin user, rather than a client.
--page int                                      Pagination: The starting page.
--pageSize int                                  Pagination: The size of the page (number of records) (default 1000)
--searchAllUsers                                if 'true', then it will search all users, in batches of 'pageSize' starting at 'page'
-t, --threads int                                   the number of threads to run the keycloak import (default 10)
-w, --url string                                    The URL of the keycloak server. (default "http://127.0.0.1:8080")
--useLegacyKeycloak                             if true, then it will use the legacy keycloak client url.
-v, --validateLoginOnly                             if true, then it will only validate the login.
--version                                       if true, Then it will show the version.
pflag: help requested
```

### Example ###

This will connect to the keycloak running locally, on port 8080, and delete all users that are older than 30 days.

```bash
kc_user_delete_older \
    --loginAsAdmin=true \
    --clientId=admin \
    --clientSecret=admin \
    --days=30  \
    --destinationRealm delete \
    --pageSize=3000 \
    --page=0 \
    --threads=6
```


## User Object ##

This tool has to pull down the user to interrogate it, as created timestamp is not something that can be searched.

The user object that is returned from keycloak looks like this:

```json
{
	"id": "63a******3e3e",
	"createdTimestamp": 1666940588880,
	"username": "testUser",
	"enabled": true,
	"totp": false,
	"emailVerified": false,
	"firstName": "Test",
	"lastName": "User",
	"email": "test_user@example.com",
	"attributes": {
			"customA": [
					"a12345"
			]
	},
	"disableableCredentialTypes": [],
	"requiredActions": [],
	"access": {
			"impersonate": true,
			"manage": true,
			"manageGroupMembership": true,
			"mapRoles": true,
			"view": true
	}
}
```
## Using Environment Variables ##

The file `delete.local.example.sh` is an example of how you could use environment variables to set the configuration.  (also seen below)

```bash
#!/usr/bin/env bash

export KC_CLIENT_ID="admin"
export KC_CLIENT_SECRET="PASSWORD"
export KC_CLIENT_REALM="master"
export KC_URL="https://my.keycloak.org"

export KC_DESTINATION_REALM="delete"
## If you only want to test, then set this to true.
export KC_DRY_RUN="true"
#export KC_DRY_RUN="false"
export KC_USERNAME="admin"
export KC_LOG_DIR="/tmp"
#export KC_LOG_CMD_VALUES=
export KC_USE_LEGACY_KEYCLOAK="true"
export KC_LOGIN_AS_ADMIN="true"
## Concurrency Settings
export KC_THREADS=10
export KC_CHANNEL_BUFFER=1000
## Deletion Date settings
#export KC_MAX_AGE_IN_DATE="2020-01-01"
## OR, but not both.
export KC_MAX_AGE_IN_DAYS=30

##  Pagination
export KC_PAGE_SIZE=7000
export KC_PAGE_OFFSET=0

# in the script you could then run: Allowing you to override the environment variables. with say --listonly 
# kc_user_delete_older "$@"
```


## Checking The Version ##

```bash
kc_user_delete_older --version

kc_user_delete_older 
 [ version=0.0.2-next ]
 [ commit=6a******f2 ]
 [ buildTime=2023-10-11T00:11:48Z ]
 ```

## Logging ##

When you call the application, it will tell you some of your config settings, watch this, as it may leak secrets.

```bash
[KeyCloak Delete via API Tool (Day/Date Based)]
  Authentication:
    clientId: admin
    clientSecret: admin
    clientRealm: master
    destinationRealm: delete
    loginAsAdmin: false
    url: http://127.0.0.1:8080
  Concurrency
    channelBuffer: 10000
    threads: 10
  Deletion Criteria
    maxDaysInAge: disabled
    deleteDate: Disabled
  Misc Config
    dryRun: false
    logCmdValues: false
    logDir: /tmp
    page: 0
    pageSize: 1000
```

## Example Call Script ##

There is a test example called [`delete.localhost.example.sh`](delete.localhost.example.sh) that you can test with.  It assumed you have built the code with `goreleaser`



### goreleaser ###

```bash
λ:> goreleaser release --snapshot --clean
```

```bash
 • starting release...
  • loading config file                              file=.goreleaser.yaml
  • loading environment variables
    • using token from "/home/paul/.config/goreleaser/github_token"
  • getting and validating git state
    • couldn't find any tags before "0.0.1"
    • building...                                    commit=6a31a2158d6b833781fd02e1f9135e1df6405ff2 latest tag=0.0.1
    • pipe skipped                                   reason=disabled during snapshot mode
  • parsing tag
  • setting defaults
  • running before hooks
    • running                                        hook=go mod tidy
    • running                                        hook=go generate ./...
  • snapshotting
    • building snapshot...                           version=0.0.2-next
  • checking distribution directory
    • cleaning dist
  • loading go mod information
  • build prerequisites
  • writing effective config file
    • writing                                        config=dist/config.yaml
  • building binaries
    • building                                       binary=dist/kc_user_delete_older_windows_arm64/kc_user_delete_older.exe
    • building                                       binary=dist/kc_user_delete_older_windows_amd64_v1/kc_user_delete_older.exe
    • building                                       binary=dist/kc_user_delete_older_linux_amd64_v1/kc_user_delete_older
    • building                                       binary=dist/kc_user_delete_older_linux_arm64/kc_user_delete_older
    • took: 1s
  • archives
    • creating                                       archive=dist/kc_user_delete_older_Windows_arm64.zip
    • creating                                       archive=dist/kc_user_delete_older_Linux_arm64.tar.gz
    • creating                                       archive=dist/kc_user_delete_older_Windows_x86_64.zip
    • creating                                       archive=dist/kc_user_delete_older_Linux_x86_64.tar.gz
    • took: 1s
  • calculating checksums
  • storing release metadata
    • writing                                        file=dist/artifacts.json
    • writing                                        file=dist/metadata.json
  • release succeeded after 2s
```


### Using Environment Variables ###

Many of the settings that are configurable via command line can be set via the OS environment variables.

The following is an example of the [`delete.localhost.example.sh`](delete.localhost.example.sh) file that you can use to set the environment variables.

(listed below for your reference)

```bash
#!/usr/bin/env bash

export KC_CLIENT_ID="admin"
export KC_CLIENT_SECRET="password"
export KC_CLIENT_REALM="master"
export KC_URL="http://localhost"

export KC_DESTINATION_REALM="test"

export KC_DRY_RUN="false"
export KC_USERNAME="admin"
export KC_LOG_DIR="/tmp"
#export KC_LOG_CMD_VALUES=
export KC_USE_LEGACY_KEYCLOAK="TRUE"
export KC_LOGIN_AS_ADMIN="true"
## Concurrency Settings
export KC_THREADS=6
export KC_CHANNEL_BUFFER=10
## Deletion Date settings
#export KC_MAX_AGE_IN_DATE="2020-01-01"
## OR, but not both.
export KC_MAX_AGE_IN_DAYS=30
## Header
export KC_HEADER_NAME="XX-HEADER-NAME"
export KC_HEADER_VALUE="header-value"

##  PAgination
export KC_PAGE_SIZE=1000
export KC_PAGE_OFFSET=0

# Listing Stuff
export KC_COUNT_ONLY="false"
export KC_LIST_ONLY="true"
```