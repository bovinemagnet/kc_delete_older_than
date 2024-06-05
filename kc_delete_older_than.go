package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/atotto/clipboard"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/golang-jwt/jwt/v5"
)

type DeletionUser struct {
	Id               string
	Username         string
	Email            string
	CreatedTimestamp int64
}

var (
	// Concurrency
	Threads       *int = flag.IntP("threads", "t", THREADS, "the number of threads to run the keycloak import")
	ChannelBuffer *int = flag.IntP("channelBuffer", "b", CHANNEL_BUFFER, "the number of buffered spaces in the channel buffer")
	// Keycloak Login Details
	ClientId     *string = flag.StringP("clientId", "u", CLIENT_ID, "The API user that will execute the calls.")
	ClientSecret *string = flag.StringP("clientSecret", "p", CLIENT_SECRET, "The secret for the keycloak user defined by `clientId`")
	ClientRealm  *string = flag.StringP("clientRealm", "s", CLIENT_REALM, "The realm in which the `clientId` exists")
	Url          *string = flag.StringP("url", "w", URL, "The URL of the keycloak server.")
	LoginAsAdmin *bool   = flag.BoolP("loginAsAdmin", "z", false, "if true, then it will login as admin user, rather than a client.")
	// Target or Destination Realm
	DestinationRealm *string = flag.StringP("destinationRealm", "d", DESTINATION_REALM, "The realm in keycloak where the users are to be created. This may or may not be the same as the `clientRealm`")
	// Options
	MaxAgeInDays *int  = flag.Int("days", EMPTY_DAYS, "the number of days, after which users are deleted")
	DryRun       *bool = flag.Bool("dryRun", false, "if true, then no users will be deleted, it will just log the outcome.")
	ShowVersion  *bool = flag.Bool("version", false, "if true, Then it will show the version.")
	// Pagination
	Page           *int  = flag.Int("page", 0, "Pagination: The starting page.")
	PageSize       *int  = flag.Int("pageSize", 1000, "Pagination: The size of the page (number of records)")
	SearchAllUsers *bool = flag.Bool("searchAllUsers", false, "if 'true', then it will search all users, in batches of 'pageSize' starting at 'page'")
	// Logging Options
	LogCmdValues        *bool   = flag.Bool("logCmdValues", false, "if true, then the command line values will be logged.")
	LogDir              *string = flag.String("logDir", os.TempDir(), "The logging directory.")
	ListOnly            *bool   = flag.Bool("listOnly", false, "if true, then it will only generate a list the users that will be deleted.")
	DeleteDate          *string = flag.String("deleteDate", "", "The date after which users will be deleted. Format: YYYY-MM-DD")
	CountTotalUsersOnly *bool   = flag.Bool("countTotalUsersOnly", false, "if true, then just  do a call to `GET /{realm}/user/count`.")
	// keycloak
	UseLegacyKeycloak *bool = flag.Bool("useLegacyKeycloak", false, "if true, then it will use the legacy keycloak client url.")
	// Validate login only
	ValidateLoginOnly *bool = flag.BoolP("validateLoginOnly", "v", false, "if true, then it will only validate the login.")
	// Headers
	HeaderKey   *string = flag.String("headerKey", "", "The header key to use for the login.")
	HeaderValue *string = flag.String("headerValue", "", "The header value to use for the login.")
	// deprecated flags
	SearchMin *int = flag.Int("searchMin", 0, "The starting number of users to search through.")
	SearchMax *int = flag.Int("searchMax", 1000, "The maximum number of users to search through.")
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// var processed uint64
var processed int32
var deleted int32

func main() {

	// Get the path to the executable file
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("[M]  Error:", err)
		return
	}

	// Get the name of the executable file
	exeName := filepath.Base(exePath)

	// Parse the env variables first, to set new defaults
	parseEnvVariables()
	// deprecate the old flags.
	flag.CommandLine.MarkDeprecated("searchMin", "use 'page' instead")
	flag.CommandLine.MarkDeprecated("searchMax", "use 'pageSize' instead")
	// Parse the command line arguments
	flag.Parse()

	if *ShowVersion {
		fmt.Printf("%s \n [ version=%s ]\n [ commit=%s ]\n [ buildTime=%s ]\n", exeName, version, commit, date)
		return
	}

	// Display the command line arguments back to the user.
	handleDryRun()
	// Check the validation is correct
	validateDayDateConfiguration()
	// log the command line arguments to the log file.
	startTimeString := strconv.FormatInt(time.Now().Unix(), 10)
	fileName, err := startLogging(exeName, startTimeString)
	if err != nil {
		fmt.Println("[M]  Unable to start logging")
		fmt.Println("[M]  Error:", err)
		//return
	}
	defer func() {
		// defer closing the log file until the end of the function
		log.Println("[M]  Closing log file:", fileName)
	}()

	startTime := makeTimestamp()

	u, _ := user.Current()

	success, err := canLogin()
	if err != nil {
		log.Println("[M]  error logging in: ", err)
		fmt.Println("[M]  FAIL: error logging in: ", err)
		return
	}
	if !success {
		log.Println("[M]  error logging in: ", err)
		return
	}
	if *ValidateLoginOnly {
		log.Println("[M]  SUCCESS: login validated.")
		fmt.Println("[M]  SUCCESS: login validated.")
		return
	}
	//
	var epoch int64
	if *MaxAgeInDays > EMPTY_DAYS {
		epoch = daysToEpoch(*MaxAgeInDays)
	} else {
		epoch, err = parseDateToEpoch(*DeleteDate)
		if err != nil {
			log.Println("[M]  error parsing date: ", err)
			fmt.Println("[M]  FAIL: error parsing date: ", err)
			return
		}
	}

	log.Println("[M] START : exe=", exeName, " epoch=", strconv.FormatInt(startTime, 10), "user=", u.Username, "olderThan=", epochToDateString(epoch), "currentDate=", epochToDateString(startTime))
	fmt.Println("[M] START : exe="+exeName+" epoch="+strconv.FormatInt(startTime, 10), " user="+u.Username, "olderThan=", epochToDateString(epoch), "currentDate=", epochToDateString(startTime))

	// If we are list only, or count then we don't need to start the workers.
	if *ListOnly || *CountTotalUsersOnly {
		log.Println("[M]       : LIST ONLY MODE")
		fmt.Println("[M]       : LIST ONLY MODE")
		listUsersByEpoch(epoch)
		return
	}
	// set up concurrency
	wgReceivers := sync.WaitGroup{}
	wgReceivers.Add(*Threads)

	usersChannel := make(chan []string, *ChannelBuffer)
	resultsChannel := make(chan string, *ChannelBuffer)
	go readUsersFromKeycloak(epoch, usersChannel)

	go writeLog(resultsChannel)

	for i := 0; i < *Threads; i++ {
		go deleteUserWorker(i, usersChannel, resultsChannel, &wgReceivers)
	}

	wgReceivers.Wait()

	endTime := makeTimestamp()
	duration := endTime - startTime
	println("[M]       : processed=" + strconv.FormatInt(int64(processed), 10))
	println("[M]       : deleted=" + strconv.FormatInt(int64(deleted), 10))
	log.Println("[M]       : processed=" + strconv.FormatInt(int64(processed), 10))
	log.Println("[M]       : deleted=" + strconv.FormatInt(int64(deleted), 10))

	println("[M]       : logging=" + fileName + " path copied to clipboard (maybe)")
	clipboard.WriteAll(fileName)
	println("[M] END   : export_success=true epoch=" + strconv.FormatInt(endTime, 10) + " duration=" + strconv.FormatInt(duration, 10) + "ms" + " processed=" + strconv.FormatInt(int64(processed), 10))
	log.Println("[M] END   : export_success=true epoch=" + strconv.FormatInt(endTime, 10) + " duration=" + strconv.FormatInt(duration, 10) + "ms" + " processed=" + strconv.FormatInt(int64(processed), 10))

}

func canLogin() (bool, error) {
	log.Println("[V][START]: Validate Login ********")

	client := initKeycloakClient()

	token, err := loginKeycloak(client)
	if err != nil {
		log.Println("[V]       : token=", token)
		log.Println("[V]       : err=", err)
		log.Println("[V][END]  : Validate Login ********")

		return false, err
	} else {
		// parse the JWT Token that came back.
		parsedToken, _, err := new(jwt.Parser).ParseUnverified(token.AccessToken, jwt.MapClaims{})
		if err != nil {
			panic(err)
		}

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if !ok {
			panic("[V] Can't parse token claims")
		}

		exp, ok := claims["exp"].(float64)
		if !ok {
			panic("[V] Can't get token expiration time")
		}

		expirationTime := time.Unix(int64(exp), 0)
		fmt.Println("[V]       : Token expires at:", expirationTime)
		log.Println("[V]       : Token expires at:", expirationTime)

		duration := time.Until(expirationTime)
		fmt.Printf("[V]       : Token will expire in: %v seconds.\n", duration)

		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		fmt.Printf("[V]       : Token will expire in: %d hours %d minutes %d seconds\n", hours, minutes, seconds)
		log.Printf("[V]       : Token will expire in: %d hours %d minutes %d seconds\n", hours, minutes, seconds)

		log.Println("[V]       : Login Validation Success", token)
		log.Println("[V][END]  : Validate Login ********")
		return true, nil
	}
}

func listUsersByEpoch(deleteEpochTime int64) {

	//func listUsersByEpoch(realmName string, clientId string, clientSecret string, targetRealm string, url string, deleteEpochTime int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()

	log.Println("[E][START]: Fetch users from keycloak ********")
	log.Println("[E]       : login")

	var client *gocloak.GoCloak
	if *UseLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(*Url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(*Url)
	}
	// set the header values if they exist.
	if (strings.TrimSpace(*HeaderKey) != "") && (strings.TrimSpace(*HeaderValue) != "") {
		client.RestyClient().Header.Set(*HeaderKey, *HeaderValue)
	}
	ctx := context.Background()
	log.Println("[E]       : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if *LoginAsAdmin {
		log.Println("[E]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, *ClientId, *ClientSecret, *ClientRealm)
	} else {
		log.Println("[E]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, *ClientId, *ClientSecret, *ClientRealm)
	}
	if err != nil {
		log.Println("[E]       : ", token)
		log.Println("[E]       : ", err)
		return
	} else {
		log.Println("[E]       : Login Success", token)
	}
	// Fetch the list of Keycloak users
	log.Println("[E]       : fetching users from keycloak")

	userParams := gocloak.GetUsersParams{}
	userParams.First = Page
	userParams.Max = PageSize
	// Count total users in keycloak.
	totalUsers, err := client.GetUserCount(ctx, token.AccessToken, *DestinationRealm, userParams)
	if err != nil {
		return
	}
	if totalUsers == 0 {
		log.Println("[O]       : No users found in system")
		log.Println("[O]       : No users In System, exiting")
		return
	} else {
		fmt.Println("[O]       : Total Users In System =", totalUsers)
		log.Println("[O]       : Total Users In System =", totalUsers)
	}
	// if we are just counting the total users then exit.
	if *CountTotalUsersOnly {
		fmt.Println("[O][END]  : counting keycloak users *******************************************")
		os.Exit(0)
	}
	// Get the keycloak users we want to be in the deletion list.
	users, err := searchKeycloakForUsers(client, token, deleteEpochTime, totalUsers)

	if err != nil {
		log.Println("[O]       : Error fetching users:", err)
		return
	}

	if len(users) > 0 {
		fmt.Println("Username,ID,CreationDate")
		log.Println("Username,ID,CreationDate")
	}
	// Parse or filter through the list of keycloak users, for the actual ones that meet the criteria for deletion
	deletionList, err := FilterKeycloakUsersByEpoch(deleteEpochTime, users, true, true)
	if err != nil {
		log.Println("[O]       : Error filtering users:", err)
		return
	}
	// If the list is returned.
	if len(deletionList) == 0 {
		fmt.Println("[O]       : No users=[0] found in the searchWindow=[", *PageSize, "] search window, older than ", epochToDateString(deleteEpochTime))
		log.Println("[O]       : No users=[0] found in the searchWindow=[", *PageSize, "]  older than ", epochToDateString(deleteEpochTime))
	}
	fmt.Println("[O]       : Identified ", len(deletionList), " users out of ", strconv.Itoa(len(users)), STRING_USERS_SEARCHED)
	fmt.Println("[O][END]  : listUsersByEpoch users *******************************************")
}

// reads file and adds data it to the channel
//func readUsersFromKeycloak(realmName string, clientId string, clientSecret string, targetRealm string, url string, deleteEpochTime int64, jobs chan []string) {

func readUsersFromKeycloak(deleteEpochTime int64, jobs chan []string) {

	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()

	log.Println("[R][START]: Fetch users from keycloak ********")
	log.Println("[R]       : login")

	var client *gocloak.GoCloak
	if *UseLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(*Url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(*Url)
	}
	// set the header values if they exist.
	if (strings.TrimSpace(*HeaderKey) != "") && (strings.TrimSpace(*HeaderValue) != "") {
		client.RestyClient().Header.Set(*HeaderKey, *HeaderValue)
	}
	ctx := context.Background()
	log.Println("[R]       : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if *LoginAsAdmin {
		log.Println("[R]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, *ClientId, *ClientSecret, *ClientRealm)
	} else {
		log.Println("[R]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, *ClientId, *ClientSecret, *ClientRealm)
	}
	if err != nil {
		log.Println("[R]       : ", token)
		log.Println("[R]       : ", err)
		close(jobs)
		return
	} else {
		log.Println("[R]       : Login Success", token)
	}
	// Fetch the list of Keycloak users
	log.Println("[R]       : fetching users from keycloak")
	userParams := gocloak.GetUsersParams{}
	userParams.Max = &*PageSize
	userParams.First = &*Page

	totalUsers, err := client.GetUserCount(ctx, token.AccessToken, *DestinationRealm, userParams)
	if err != nil {
		return
	}
	log.Println("[R]       : Total Users In System =", totalUsers)
	fmt.Println("[R]       : Total Users In System =", totalUsers)

	// get the keycloak users we want to be in the deletion list.
	kcUsers, err := searchKeycloakForUsers(client, token, deleteEpochTime, totalUsers)
	//users, err := client.GetUsers(ctx, token.AccessToken, *DestinationRealm, userParams)
	if err != nil {
		//fmt.Println("Error fetching users:", err)
		log.Println("[R]       : Error fetching users:", err)
		close(jobs)
		return
	}

	// Parse or filter through the list of keycloak users, for the actual ones that meet the criteria for deletion
	users, err := FilterKeycloakUsersByEpoch(deleteEpochTime, kcUsers, false, true)
	if err != nil {
		log.Println("[R]       : Error filtering users err:", err)
		close(jobs)
		return
	}
	fmt.Println("[R]       : Found ", len(users), " users to delete out of ", strconv.Itoa(len(kcUsers)), STRING_USERS_SEARCHED)
	var counter int32 = 0

	// Delete users that were created more than 7 days ago
	log.Println("[R]       : adding user to deletion queue")
	for _, user := range users {
		if deleteEpochTime >= user.CreatedTimestamp {
			// Add the user to the deletion queue
			jobs <- []string{user.Username, user.Id}
			counter++
		}
	}
	log.Println("[R]       : Added ", counter, " users to deletion queue out of ", strconv.Itoa(len(users)), STRING_USERS_SEARCHED)

	log.Println("[R][END]  : reading keycloak users *******************************************")

	// As there are many threads adding to the counter, lets do the addition atomically
	atomic.AddInt32(&processed, counter)

	close(jobs)
}

func writeLog(results chan string) {
	for j := range results {
		log.Println("[L] RSLT  : ", j)
	}
}

// func deleteUserWorker(id int, realmName string, clientId string, clientSecret string, targetRealm string, url string, dryRun bool, loginAsAdmin bool, jobs <-chan []string, results chan<- string, wg *sync.WaitGroup) {
func deleteUserWorker(id int, jobs <-chan []string, results chan<- string, wg *sync.WaitGroup) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[D] panic : ", r.(string))
		}
	}()

	defer wg.Done()

	successCounter := 0
	log.Println("[D][", id, "]  : Bulk User Tool Starting")

	var client *gocloak.GoCloak
	if *UseLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(*Url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(*Url)
	}
	// set the header values if they exist.
	if (strings.TrimSpace(*HeaderKey) != "") && (strings.TrimSpace(*HeaderValue) != "") {
		client.RestyClient().Header.Set(*HeaderKey, *HeaderValue)
	}
	ids := strconv.Itoa(id)
	ctx := context.Background()
	log.Println("[D][", ids, "]  : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if *LoginAsAdmin {
		log.Println("[D]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, *ClientId, *ClientSecret, *ClientRealm)
	} else {
		log.Println("[D]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, *ClientId, *ClientSecret, *ClientRealm)
	}
	if err != nil {
		log.Println("[D][", ids, "] ", token)
		log.Println("[D][", ids, "] ", err)
		log.Println("[D][", ids, "] clientId=", *ClientId)
		fmt.Println("[D][", ids, "] clientId=["+*ClientId+"]")
		// because of this error, we will signal the worker group that this worker is done
		//defer wg.Done()
		return
		//panic("[D][" + ids + "] Something wrong with the credentials or url : Error: " + err.Error())
	}

	for channelData := range jobs {
		ok := true

		log.Println("[D][", ids, "]  : Looking for ", channelData[0])
		users, err := client.GetUsers(
			ctx,
			token.AccessToken,
			*DestinationRealm,
			gocloak.GetUsersParams{
				Username: &channelData[0]})
		if err != nil {

			if err.Error() == "401 Unauthorized: HTTP 401 Unauthorized" {
				// if we get a 401, then we need to re-login.
				log.Println("[C][", ids, "] : refresh token attempt")
				fmt.Println("[C][", ids, "] : refresh token attempt")
				//wg.Done()
				// try to refresh the token
				newToken, err := client.RefreshToken(ctx, token.RefreshToken, *ClientId, *ClientSecret, *ClientRealm)
				if err != nil {
					//panic(err)
					log.Println("[C][", ids, "] : exiting thread due to 401, refresh token failed "+err.Error())
					fmt.Println("[C][", ids, "] : exiting thread due to 401, refresh token failed "+err.Error())
					return
				} else {
					log.Println("[C][", ids, "] : refresh token success "+newToken.AccessToken)
				}
			} else {
				panic("[D][" + ids + "] user=" + channelData[0] + " Delete users failed. error=" + err.Error())
			}
		}
		userID := ""
		for _, userFnd := range users {
			log.Println("[D][", ids, "]  : FOUND ", *userFnd.ID, " ", *userFnd.Username)
			userID = *userFnd.ID
		}
		if userID == "" {
			log.Println("[D][", ids, "] ", channelData[0], "User not found")
			results <- "[" + ids + "] " + channelData[0] + " User not found"

		} else {

			if !*DryRun {
				err := client.DeleteUser(ctx, token.AccessToken, *DestinationRealm, userID)
				if err != nil {
					log.Println("[D][", ids, "] delete user error : ", err.Error())
					ok = false
					//panic("Oh no!, failed to create user :(")
					// set the return error code.
					results <- "[D][" + ids + "] " + channelData[0] + " " + userID + " " + err.Error()
				} else {
					// if we need more logging.
					//log.Println(ids, "]deleted user success : ", createdUser)
					ok = true
					deleted++
				}
			} else {
				results <- "[D][" + ids + "] " + channelData[0] + " " + userID + " dry run"
				ok = true
			}
		}

		// if we get to the end, and it is still ok, then we assume the user is created
		// and we add the return message.
		if ok {
			successCounter++
			results <- "[D][" + ids + "] " + channelData[0] + "  : successfully deleted"
		}
	}

	log.Println("[D][", ids, "]  : deleted ", successCounter, " users")
}

func makeTimestamp() int64 {
	return time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

func nowAsUnixMilliseconds() int64 {
	return time.Now().Round(time.Millisecond).UnixNano() / 1e6
}

// Print the command line arguments on the screen.
func printCmdLineArgsCfg() {
	outputCmdLineArgsCfg(os.Stdout)
}

// Send the command line arguments to the log file
func logCmdLineArgs() {
	// Prepare a writer from logger. This assumes that log.Output() is your logger instance.
	loggerWriter := log.Writer()
	outputCmdLineArgsCfg(loggerWriter)
}

func testColour() {
	fmt.Println(string(colorRed), "test", string(colorReset))
	fmt.Println(string(colorGreen), "test", string(colorReset))
	fmt.Println(string(colorYellow), "test", string(colorReset))
	fmt.Println(string(colorBlue), "test", string(colorReset))
	fmt.Println(string(colorPurple), "test", string(colorReset))
	fmt.Println(string(colorWhite), "test", string(colorReset))
	fmt.Println(string(colorCyan), "test", string(colorReset))
	fmt.Println("next")
}

// go run bulk-user-delete.go -destinationRealm deleteme

func daysSinceCreation(createdAt int64) int {
	// Calculate the number of days since the user was created
	now := time.Now().Unix()
	ageInSeconds := now - (createdAt / 1000)
	return int(ageInSeconds / 86400)
}

func daysSinceCreationAtTime(createdAt int64, now int64) int {
	// Calculate the number of days since the user was created
	//now := time.Now().Unix()
	ageInSeconds := now - (createdAt / 1000)
	return int(ageInSeconds / 86400)
}

// convert days to epoc
func daysToEpoch(days int) int64 {
	return time.Now().AddDate(0, 0, -days).UnixNano() / int64(time.Millisecond)
}

// convert time to epoch
func timeToEpoch(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func parseDate(dateString string) (time.Time, error) {
	date, err := time.Parse(DateFormat, dateString)
	if err != nil {
		return time.Now(), err
	}
	return date, nil
}

func parseDateToEpoch(dateString string) (int64, error) {
	date, err := time.Parse(DateFormat, dateString)
	if err != nil {
		return 0, err
	}
	return timeToEpoch(date), nil
}

// Parse days to string date
func daysToDate(days int) string {
	return time.Now().AddDate(0, 0, -days).Format(DateFormat)
}

func subtractDaysToDate(days int, time time.Time) string {
	return time.AddDate(0, 0, -days).Format(DateFormat)
}

func epochToDateString(epoch int64) string {
	return time.Unix(0, epoch*int64(time.Millisecond)).Format(DateFormat)
}

/*
 * Parse the environmental variables, this will be overridden by the command line arguments.
 */
func parseEnvVariables() {
	// Parsing code, setting config fields from environment variables.
	envVars := map[string]*string{
		ENV_CLIENT_ID:         &*ClientId,
		ENV_CLIENT_SECRET:     &*ClientSecret,
		ENV_CLIENT_REALM:      &*ClientRealm,
		ENV_DESTINATION_REALM: &*DestinationRealm,
		ENV_URL:               &*Url,
		ENV_DELETE_DATE:       &*DeleteDate,
		ENV_HEADER_NAME:       &*HeaderKey,
		ENV_HEADER_VALUE:      &*HeaderValue,
		ENV_LOG_DIR:           &*LogDir,
	}

	envBools := map[string]*bool{
		ENV_DRY_RUN:             &*DryRun,
		ENV_LOG_CMD_VALUES:      &*LogCmdValues,
		ENV_USE_LEGACY_KEYCLOAK: &*UseLegacyKeycloak,
		ENV_LOGIN_AS_ADMIN:      &*LoginAsAdmin,
		ENV_SEARCH_ALL_USERS:    &*SearchAllUsers,
		ENV_COUNT_ONLY:          &*CountTotalUsersOnly,
		ENV_LIST_ONLY:           &*ListOnly,
	}

	envInts := map[string]*int{
		ENV_THREADS:         &*Threads,
		ENV_CHANNEL_BUFFER:  &*ChannelBuffer,
		ENV_MAX_AGE_IN_DAYS: &*MaxAgeInDays,
		ENV_PAGE_SIZE:       &*PageSize,
		ENV_PAGE_OFFSET:     &*Page,
	}

	for envVar, field := range envVars {
		if value := os.Getenv(envVar); value != "" {
			*field = value
		}
	}

	for envVar, field := range envBools {
		if value := os.Getenv(envVar); value != "" {
			*field = strings.ToLower(value) == "true"
		}
	}

	for envVar, field := range envInts {
		if value := os.Getenv(envVar); value != "" {
			parsedValue, err := strconv.Atoi(value)
			if err != nil {
				log.Fatalf("Error parsing integer from env variable %s: %v", envVar, err)
			}
			*field = parsedValue
		}
	}
}

func validateDayDateConfiguration() {
	if *MaxAgeInDays > EMPTY_DAYS && *DeleteDate != "" {
		log.Fatalf("Error: maxAgeInDays and deleteDate are both set. Please set only one of them.")
	}

	if *MaxAgeInDays <= EMPTY_DAYS && *DeleteDate == "" {
		log.Fatalf("Error: maxAgeInDays and deleteDate are both not set. Please set one of them.")
	}

	if *DeleteDate != "" {
		_, err := time.Parse(DateFormat, *DeleteDate)
		if err != nil {
			log.Fatalf("Error: deleteDate is not in the correct format. Please use YYYY-MM-DD")
		}
	}
}

func initKeycloakClient() *gocloak.GoCloak {
	var client *gocloak.GoCloak
	if *UseLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(*Url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(*Url)
	}
	// Add the custom header set, if configured.
	if strings.TrimSpace(*HeaderKey) != "" && strings.TrimSpace(*HeaderValue) != "" {
		client.RestyClient().Header.Set(*HeaderKey, *HeaderValue)
	}

	return client
}

func loginKeycloak(client *gocloak.GoCloak) (*gocloak.JWT, error) {
	ctx := context.Background()
	var token *gocloak.JWT
	var err error
	if *LoginAsAdmin {
		token, err = client.LoginAdmin(ctx, *ClientId, *ClientSecret, *ClientRealm)
	} else {
		token, err = client.LoginClient(ctx, *ClientId, *ClientSecret, *ClientRealm)
	}
	return token, err
}

func startLogging(exeName string, startTimeString string) (string, error) {

	f, err := os.OpenFile(filepath.Join(*LogDir, startTimeString+"-"+exeName+".log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
		return "", err
	}
	log.SetOutput(f)
	log.SetFlags(0)
	logCmdLineArgs()

	log.Println("Logging started")
	//defer f.Close() // Replaced this with a func defer in main.
	return f.Name(), nil
}

// Function that accepts an io.Writer to print or log
func outputCmdLineArgsCfg(out io.Writer) {
	_, _ = fmt.Fprintln(out, "[KeyCloak Delete via API Tool (Day/Date Based)]")
	_, _ = fmt.Fprintln(out, "  Authentication:")
	_, _ = fmt.Fprintln(out, "    clientId:", *ClientId)
	_, _ = fmt.Fprintln(out, "    clientSecret:", *ClientSecret)
	_, _ = fmt.Fprintln(out, "    clientRealm:", *ClientRealm)
	_, _ = fmt.Fprintln(out, "    destinationRealm:", *DestinationRealm)
	_, _ = fmt.Fprintln(out, "    loginAsAdmin:", *LoginAsAdmin)
	_, _ = fmt.Fprintln(out, "    url:", *Url)
	_, _ = fmt.Fprintln(out, "    useLegacyKeycloak:", *UseLegacyKeycloak)
	_, _ = fmt.Fprintln(out, "  Concurrency")
	_, _ = fmt.Fprintln(out, "    channelBuffer:", *ChannelBuffer)
	_, _ = fmt.Fprintln(out, "    threads:", *Threads)
	_, _ = fmt.Fprintln(out, "  Deletion Criteria")

	if *MaxAgeInDays > 0 {
		_, _ = fmt.Fprintln(out, "    maxDaysInAge:", *MaxAgeInDays, "[", daysToDate(*MaxAgeInDays), "]")
	} else {
		_, _ = fmt.Fprintln(out, "    maxDaysInAge:", "disabled")
	}
	if *DeleteDate != "" {
		_, _ = fmt.Fprintln(out, "    deleteDate:", *DeleteDate)
	} else {
		_, _ = fmt.Fprintln(out, "    deleteDate:", "Disabled")
	}
	_, _ = fmt.Fprintln(out, "  Misc Config")
	_, _ = fmt.Fprintln(out, "    dryRun:", *DryRun)
	_, _ = fmt.Fprintln(out, "    logCmdValues:", *LogCmdValues)
	_, _ = fmt.Fprintln(out, "    logDir:", *LogDir)
	_, _ = fmt.Fprintln(out, "    page:", *Page)
	_, _ = fmt.Fprintln(out, "    pageSize:", *PageSize)
	_, _ = fmt.Fprintln(out, "    searchAllUsers:", *SearchAllUsers)
	_, _ = fmt.Fprintln(out, "    countTotalUsersOnly:", *CountTotalUsersOnly)
	_, _ = fmt.Fprintln(out, "    listOnly:", *ListOnly)
	_, _ = fmt.Fprintln(out, " ")
}

func handleDryRun() {
	if *DryRun {
		log.Println("Dry run mode enabled. No users will be deleted.")
		printCmdLineArgsCfg()
	}
}

/**
 * Filter the Keycloak users by the epoch time.
 * return a list of users that meet the criteria.
 */
func FilterKeycloakUsersByEpoch(epoch int64, users []*gocloak.User, printOut bool, logOut bool) ([]DeletionUser, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()
	// Check if users is empty
	if len(users) == 0 {
		if logOut {
			log.Println("[F]       : No users found in system")
			log.Println("[F]       : No users In System, exiting")
		}
		return nil, nil
	}
	// Create a list of users to delete
	deletionUsers := make([]DeletionUser, 0, 2)
	if logOut {
		log.Println("[F]       : filtering through ", len(users), " users")
	}
	if printOut {
		fmt.Println("[F]       : filtering through ", len(users), " users")
	}
	for _, kcUser := range users {

		if epoch >= *kcUser.CreatedTimestamp {
			// Add the user to the deletion queue
			if printOut {
				fmt.Println(*kcUser.Username, ",", *kcUser.ID, ",", epochToDateString(*kcUser.CreatedTimestamp))
			}
			if logOut {
				log.Println(*kcUser.Username, ",", *kcUser.ID, ",", epochToDateString(*kcUser.CreatedTimestamp))
			}
			deletionUsers = append(deletionUsers, DeletionUser{Username: *kcUser.Username, Id: *kcUser.ID, CreatedTimestamp: *kcUser.CreatedTimestamp, Email: *kcUser.Email})
		}
	}

	if logOut {
		log.Println("[F]       : Identified ", len(deletionUsers), " users out of ", strconv.Itoa(len(users)), STRING_USERS_SEARCHED)
	}
	if printOut {
		fmt.Println("[F]       : Identified ", len(deletionUsers), " users out of ", strconv.Itoa(len(users)), STRING_USERS_SEARCHED)
	}
	return deletionUsers, nil
}

/**
 * Get the keycloak users from keycloak.
 */
func searchKeycloakForUsers(client *gocloak.GoCloak, token *gocloak.JWT, epoch int64, totalUsers int) ([]*gocloak.User, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()
	//var users []*gocloak.User
	users := make([]*gocloak.User, 0, 2)

	ctx := context.Background()
	userParams := gocloak.GetUsersParams{}
	userParams.First = Page
	userParams.Max = PageSize
	if *SearchAllUsers {
		// Build the user list.

		for currentPage := *Page; currentPage**PageSize < totalUsers; currentPage++ {
			startIndex := currentPage * *PageSize
			endIndex := (currentPage + 1) * *PageSize
			if endIndex > totalUsers {
				endIndex = totalUsers
			}
			userParams.First = &startIndex
			userParams.Max = PageSize
			fmt.Printf("Building User List: user page=%d: users from=%d to=%d", currentPage, startIndex+1, endIndex)
			tmpUsers, err := client.GetUsers(ctx, token.AccessToken, *DestinationRealm, userParams)
			if err != nil {
				log.Println("[O]       : Error fetching users:", err, " total_users_fetched=", len(users))
				return nil, err
			}
			// Add the users to the list
			users = append(users, tmpUsers...)
			fmt.Printf("[K] total_users_fetched=%d\n", len(users))
		}
	} else {
		// just add the ones that are in the search window
		tmpUsers, err := client.GetUsers(ctx, token.AccessToken, *DestinationRealm, userParams)

		//	users, err := client.GetUsers(ctx, token.AccessToken, targetRealm, gocloak.GetUsersParams{})
		if err != nil {
			//fmt.Println("Error fetching users:", err)
			log.Println("[K]       : Error fetching users:", err)
			return nil, err
		}
		users = append(users, tmpUsers...)
		// Delete users that were created more than 7 days ago
		log.Println("[K]       : adding user to deletion queue")
		// get the count of users

	}
	return users, nil
}
