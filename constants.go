package main

const (
	THREADS        = 10
	CHANNEL_BUFFER = 10000
	// admin-cli user
	//CLIENT_ID         = "admin-cli"
	//CLIENT_SECRET     = "tCDmV1XDJ6jV8g37ac6fxVhBP9AmIbG3"
	// admin login user
	CLIENT_ID         = "admin"
	CLIENT_SECRET     = "admin"
	CLIENT_REALM      = "master"
	DESTINATION_REALM = "delete"
	URL               = "http://127.0.0.1:8080"
	MAX_AGE_IN_DAYS   = 30
	DRY_RUN           = true
	EMPTY_DAYS        = -1
)

// Environment variables
const (
	ENV_CLIENT_ID           = "KC_CLIENT_ID"
	ENV_CLIENT_REALM        = "KC_CLIENT_REALM"
	ENV_CLIENT_SECRET       = "KC_CLIENT_SECRET"
	ENV_URL                 = "KC_URL"
	ENV_DESTINATION_REALM   = "KC_DESTINATION_REALM"
	ENV_DRY_RUN             = "KC_DRY_RUN"
	ENV_USERNAME            = "KC_USERNAME"
	ENV_LOG_DIR             = "KC_LOG_DIR"
	ENV_LOG_CMD_VALUES      = "KC_LOG_CMD_VALUES"
	ENV_USE_LEGACY_KEYCLOAK = "KC_USE_LEGACY_KEYCLOAK"
	ENV_LOGIN_AS_ADMIN      = "KC_LOGIN_AS_ADMIN"
	// concurrency
	ENV_THREADS        = "KC_THREADS"
	ENV_CHANNEL_BUFFER = "KC_CHANNEL_BUFFER"
	// Deletion on days.
	ENV_MAX_AGE_IN_DATE = "KC_MAX_AGE_IN_DATE"
	ENV_MAX_AGE_IN_DAYS = "KC_MAX_AGE_IN_DAYS"
	// Pagination
	ENV_PAGE_SIZE   = "KC_PAGE_SIZE"
	ENV_PAGE_OFFSET = "KC_PAGE_OFFSET"
	// Header
	ENV_HEADER_NAME  = "KC_HEADER_NAME"
	ENV_HEADER_VALUE = "KC_HEADER_VALUE"
)

// Output colours.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Misc other constants.
const (
	// Date format
	DateFormat = "2006-01-02"
)
