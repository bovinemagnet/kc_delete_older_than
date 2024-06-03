#!/usr/bin/env bash

export KC_CLIENT_ID="admin"
export KC_CLIENT_SECRET="admin"
export KC_CLIENT_REALM="master"
export KC_URL="http://127.0.0.1:8080"

#export KC_DESTINATION_REALM="npe"
export KC_DESTINATION_REALM="delete"

export KC_DRY_RUN="false"
export KC_USERNAME="admin"
export KC_LOG_DIR="/tmp"
#export KC_LOG_CMD_VALUES=
export KC_USE_LEGACY_KEYCLOAK="true"
export KC_LOGIN_AS_ADMIN="true"
## Concurrency Settings
export KC_THREADS=2
export KC_CHANNEL_BUFFER=10
## Deletion Date settings
#export KC_MAX_AGE_IN_DATE="2020-01-01"
## OR, but not both.
export KC_MAX_AGE_IN_DAYS=30
## Header
export KC_HEADER_NAME="X-Delete-Older-Than"
export KC_HEADER_VALUE="true"

##  PAgination
export KC_PAGE_SIZE=5000
export KC_PAGE_OFFSET=0

./kc_delete_older_than "$@"
