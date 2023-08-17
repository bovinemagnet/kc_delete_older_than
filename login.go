package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/dgrijalva/jwt-go"
)

func login(clientRealmName string, clientId string, clientSecret string, url string, headerName string, headerValue string, loginAsAdmin *bool, validateLogin *bool) (*gocloak.GoCloak, *gocloak.JWT, error) {
	output(ERROR, true, false, "[L][START]: Login ********")

	var client *gocloak.GoCloak
	if *useLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(url)
	}
	if (headerName != "") && (headerValue != "") {
		client.RestyClient().Header.Set(headerName, headerValue)
	}
	ctx := context.Background()
	var token *gocloak.JWT
	var err error
	if *loginAsAdmin {
		log.Println("[L]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, clientId, clientSecret, clientRealmName)
	} else {
		log.Println("[L]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, clientId, clientSecret, clientRealmName)
	}
	if err != nil {
		output(ERROR, true, false, "[L]       : token=%s", token)
		output(ERROR, true, false, "[L]       : err=%s", err)
		output(ERROR, true, false, "[L][END]  : Login ********")

		//return false, err
		return nil, nil, err

	} else {
		// parse the JWT Token that came back.
		parsedToken, _, err := new(jwt.Parser).ParseUnverified(token.AccessToken, jwt.MapClaims{})
		if err != nil {
			output(ERROR, true, false, "[L]       : err=%s", err)
			panic(err)
		}

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if !ok {
			output(ERROR, true, false, "[L]       : Can't parse token claims. err=%s", err)
			panic("[L] Can't parse token claims")
		}

		exp, ok := claims["exp"].(float64)
		if !ok {
			output(ERROR, true, false, "[L]       : Can't get token expiration time")
			panic("[L] Can't get token expiration time")
		}
		if *validateLogin {
			output(INFO, true, true, "[L]       : Login successful")
		}

		expirationTime := time.Unix(int64(exp), 0)
		output(INFO, true, true, "[L]       : Token expires at: %s", expirationTime.String())

		duration := time.Until(expirationTime)
		fmt.Printf("[L]        : Token will expire in: %v seconds.\n", duration)

		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		output(INFO, true, true, "[L]        : Token will expire in: %d hours %d minutes %d seconds\n", hours, minutes, seconds)

		output(INFO, true, false, "[L]       : Login Validation Success. token=%s", token)
		output(INFO, true, false, "[L][END]  : Validate Login ********")
		return client, token, nil
	}
}
