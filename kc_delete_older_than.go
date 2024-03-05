package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/atotto/clipboard"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/dgrijalva/jwt-go"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	// Concurrency
	threads       *int = flag.IntP("threads", "t", THREADS, "the number of threads to run the keycloak import")
	channelBuffer *int = flag.IntP("channelBuffer", "b", CHANNEL_BUFFER, "the number of buffered spaces in the channel buffer")
	// Keycloak Login Details
	clientId     *string = flag.StringP("clientId", "u", CLIENT_ID, "The API user that will execute the calls.")
	clientSecret *string = flag.StringP("clientSecret", "p", CLIENT_SECRET, "The secret for the keycloak user defined by `clientId`")
	clientRealm  *string = flag.StringP("clientRealm", "s", CLIENT_REALM, "The realm in which the `clientId` exists")
	url          *string = flag.StringP("url", "w", URL, "The URL of the keycloak server.")
	loginAsAdmin *bool   = flag.BoolP("loginAsAdmin", "z", false, "if true, then it will login as admin user, rather than a client.")
	// Target or Destination Realm
	destinationRealm *string = flag.StringP("destinationRealm", "d", DESTINATION_REALM, "The realm in keycloak where the users are to be created. This may or may not be the same as the `clientRealm`")
	// Options
	maxAgeInDays *int  = flag.Int("days", EMPTY_DAYS, "the number of days, after which users are deleted")
	dryRun       *bool = flag.Bool("dryRun", false, "if true, then no users will be deleted, it will just log the outcome.")
	showVersion  *bool = flag.Bool("version", false, "if true, Then it will show the version.")

	// Logging Options
	logCmdValues *bool   = flag.Bool("logCmdValues", false, "if true, then the command line values will be logged.")
	logDir       *string = flag.String("logDir", os.TempDir(), "The logging directory.")
	listOnly     *bool   = flag.Bool("listOnly", false, "if true, then it will only generate a list the users that will be deleted.")
	deleteDate   *string = flag.String("deleteDate", "", "The date after which users will be deleted. Format: YYYY-MM-DD")
	searchMin    *int    = flag.Int("searchMin", 0, "The starting number of users to search through.")
	searchMax    *int    = flag.Int("searchMax", 1000, "The maximum number of users to search through.")
	// keycloak
	useLegacyKeycloak *bool = flag.Bool("useLegacyKeycloak", false, "if true, then it will use the legacy keycloak client url.")
	// Validate login only
	validateLoginOnly *bool = flag.BoolP("validateLoginOnly", "v", false, "if true, then it will only validate the login.")
	// Headers
	headerKey   *string = flag.String("headerKey", "", "The header key to use for the login.")
	headerValue *string = flag.String("headerValue", "", "The header value to use for the login.")
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

	// Parse the env variables
	parseEnvVariables()

	// Parse the command line arguments
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s \n [ version=%s ]\n [ commit=%s ]\n [ buildTime=%s ]\n", exeName, version, commit, date)
		return
	}

	// Display the command line arguments back to the user.
	if dryRun != nil && *dryRun {
		printCmdLineArgs()
	}
	// if maxAgeInDays and date are both set, then we need to exit.
	if *maxAgeInDays > EMPTY_DAYS && *deleteDate != "" {
		fmt.Println("[M]  Error: maxAgeInDays and deleteDate are both set. Please set only one of them.")
		return
	}

	// check if neither are set.
	if *maxAgeInDays <= EMPTY_DAYS && *deleteDate == "" {
		fmt.Println("[M]  Error: maxAgeInDays and deleteDate are both not set. Please set only one of them.")
		return
	}

	// Check if the date is set, and if so, if it can be parsed.
	if *deleteDate != "" {
		_, err := time.Parse(DateFormat, *deleteDate)
		if err != nil {
			fmt.Println("[M]  Error: deleteDate is not in the correct format. Please use YYYY-MM-DD")
			return
		}
	}

	// log the command line arguments to the log file.

	startTimeString := strconv.FormatInt(time.Now().Unix(), 10)

	startTime := makeTimestamp()

	f, err := os.OpenFile(*logDir+"/"+startTimeString+"-"+exeName+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("[M]  error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)

	rand.Seed(time.Now().UnixNano())
	log.SetFlags(0)
	logCmdLineArgs()

	u, _ := user.Current()

	success, err := canLogin(*clientRealm, *clientId, *clientSecret, *url, *headerKey, *headerValue)
	if err != nil {
		log.Println("[M]  error logging in: ", err)
		fmt.Println("[M]  FAIL: error logging in: ", err)
		return
	}
	if !success {
		log.Println("[M]  error logging in: ", err)
		return
	}
	if *validateLoginOnly {
		log.Println("[M]  SUCCESS: login validated.")
		fmt.Println("[M]  SUCCESS: login validated.")
		return
	}
	//
	var epoch int64
	if *maxAgeInDays > EMPTY_DAYS {
		epoch = daysToEpoch(*maxAgeInDays)
	} else {
		epoch, err = parseDateToEpoch(*deleteDate)
		if err != nil {
			log.Println("[M]  error parsing date: ", err)
			fmt.Println("[M]  FAIL: error parsing date: ", err)
			return
		}
	}

	log.Println("[M] START : exe=", exeName, " epoch=", strconv.FormatInt(startTime, 10), "user=", u.Username, "olderThan=", epochToDateString(epoch), "currentDate=", epochToDateString(startTime))
	fmt.Println("[M] START : exe="+exeName+" epoch="+strconv.FormatInt(startTime, 10), " user="+u.Username, "olderThan=", epochToDateString(epoch), "currentDate=", epochToDateString(startTime))

	// If we are list only, then we don't need to start the workers.
	if *listOnly {
		log.Println("[M]       : LIST ONLY MODE")
		fmt.Println("[M]       : LIST ONLY MODE")
		listUsersByEpoch(*clientRealm, *clientId, *clientSecret, *destinationRealm, *url, epoch)
		return
	}

	wgReceivers := sync.WaitGroup{}
	wgReceivers.Add(*threads)

	usersChannel := make(chan []string, *channelBuffer)
	resultsChannel := make(chan string, *channelBuffer)
	go readUsersFromKeycloak(*clientRealm, *clientId, *clientSecret, *destinationRealm, *url, epoch, usersChannel)

	go writeLog(resultsChannel)

	for i := 0; i < *threads; i++ {
		go deleteUserWorker(i, *clientRealm, *clientId, *clientSecret, *destinationRealm, *url, *dryRun, *loginAsAdmin, usersChannel, resultsChannel, &wgReceivers)
	}

	wgReceivers.Wait()

	endTime := makeTimestamp()
	duration := endTime - startTime
	println("[M]       : processed=" + strconv.FormatInt(int64(processed), 10))
	println("[M]       : deleted=" + strconv.FormatInt(int64(deleted), 10))
	println("[M]       : logging=" + f.Name() + " path copied to clipboard (maybe)")
	clipboard.WriteAll(f.Name())
	println("[M] END   : export_success=true epoch=" + strconv.FormatInt(endTime, 10) + " duration=" + strconv.FormatInt(duration, 10) + "ms" + " processed=" + strconv.FormatInt(int64(processed), 10))

}

func canLogin(clientRealmName string, clientId string, clientSecret string, url string, headerName string, headerValue string) (bool, error) {
	log.Println("[V][START]: Validate Login ********")

	var client *gocloak.GoCloak
	if *useLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(url)
	}
	client.RestyClient().Header.Set(headerName, headerValue)
	ctx := context.Background()
	var token *gocloak.JWT
	var err error
	if *loginAsAdmin {
		log.Println("[V]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, clientId, clientSecret, clientRealmName)
	} else {
		log.Println("[V]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, clientId, clientSecret, clientRealmName)
	}
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
		fmt.Printf("[V]        : Token will expire in: %v seconds.\n", duration)

		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		fmt.Printf("[V]        : Token will expire in: %d hours %d minutes %d seconds\n", hours, minutes, seconds)
		log.Printf("[V]        : Token will expire in: %d hours %d minutes %d seconds\n", hours, minutes, seconds)

		log.Println("[V]       : Login Validation Success", token)
		log.Println("[V][END]  : Validate Login ********")
		return true, nil
	}
}

func listUsersByEpoch(realmName string, clientId string, clientSecret string, targetRealm string, url string, deleteEpochTime int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()

	log.Println("[O][START]: Fetch users from keycloak ********")
	log.Println("[O]       : login")

	var client *gocloak.GoCloak
	if *useLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(url)
	}
	if headerKey != nil && *headerKey != "" {
		// set the values if they exist.
		client.RestyClient().Header.Set(*headerKey, *headerKey)
	}
	ctx := context.Background()
	log.Println("[O]       : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if *loginAsAdmin {
		log.Println("[O]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, clientId, clientSecret, realmName)
	} else {
		log.Println("[O]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, clientId, clientSecret, realmName)
	}
	if err != nil {
		log.Println("[O]       : ", token)
		log.Println("[O]       : ", err)
		return
	} else {
		log.Println("[O]       : Login Success", token)
	}
	// Fetch the list of Keycloak users
	log.Println("[O]       : fetching users from keycloak")

	userParams := gocloak.GetUsersParams{}
	userParams.First = searchMin
	userParams.Max = searchMax
	//searchIdp := "e"
	//userParams.IDPUserID = &searchIdp -- for exact match
	//userParams.Search = &searchIdp // Will match username, first, last or email

	totalUsers, err := client.GetUserCount(ctx, token.AccessToken, targetRealm, userParams)
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

	users, err := client.GetUsers(ctx, token.AccessToken, targetRealm, userParams)

	//	users, err := client.GetUsers(ctx, token.AccessToken, targetRealm, gocloak.GetUsersParams{})
	if err != nil {
		//fmt.Println("Error fetching users:", err)
		log.Println("[O]       : Error fetching users:", err)
		return
	}

	var counter int32 = 0
	// Delete users that were created more than 7 days ago
	log.Println("[O]       : adding user to deletion queue")
	// get the count of users
	if len(users) > 0 {
		fmt.Println("Username,ID")
		log.Println("Username,ID")
	}

	for _, user := range users {

		//fmt.Println("[O] user: ", user)
		//fmt.Println("[O] user createdTS: ", *user.CreatedTimestamp)

		// if days are set to -

		if deleteEpochTime >= *user.CreatedTimestamp {
			// Add the user to the deletion queue
			fmt.Println(*user.Username, ",", *user.ID)
			log.Println(*user.Username, ",", *user.ID)
			counter++
		}
	}
	log.Println("[O]       : Identified ", counter, " users out of ", strconv.Itoa(len(users)), " users searched")
	log.Println("[O][END]  : reading keycloak users *******************************************")
	fmt.Println("[O]       : Identified ", counter, " users out of ", strconv.Itoa(len(users)), " users searched")
	fmt.Println("[O][END]  : reading keycloak users *******************************************")

}

// reads file and adds data it to the channel
func readUsersFromKeycloak(realmName string, clientId string, clientSecret string, targetRealm string, url string, deleteEpochTime int64, jobs chan []string) {

	defer func() {
		if r := recover(); r != nil {
			log.Println("[PANIC]: ", r.(string))
			println("panic:" + r.(string))
		}
	}()

	log.Println("[R][START]: Fetch users from keycloak ********")
	log.Println("[R]       : login")

	var client *gocloak.GoCloak
	if *useLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(url)
	}
	if (headerKey != nil && *headerKey != "") && (headerValue != nil && *headerValue != "") {
		client.RestyClient().Header.Set(*headerKey, *headerValue)
	}
	ctx := context.Background()
	log.Println("[R]       : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if *loginAsAdmin {
		log.Println("[R]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, clientId, clientSecret, realmName)
	} else {
		log.Println("[R]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, clientId, clientSecret, realmName)
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
	userParams.Max = searchMax
	userParams.First = searchMin

	totalUsers, err := client.GetUserCount(ctx, token.AccessToken, targetRealm, userParams)
	if err != nil {
		return
	}
	log.Println("[R]       : Total Users In System =", totalUsers)
	fmt.Println("[R]       : Total Users In System =", totalUsers)

	users, err := client.GetUsers(ctx, token.AccessToken, targetRealm, userParams)
	if err != nil {
		//fmt.Println("Error fetching users:", err)
		log.Println("[R]       : Error fetching users:", err)
		close(jobs)
		return
	}

	var counter int32 = 0

	// Delete users that were created more than 7 days ago
	log.Println("[R]       : adding user to deletion queue")
	for _, user := range users {
		//fmt.Println("[R] User", user)
		//ageInDays := daysSinceCreation(*user.CreatedTimestamp)
		if deleteEpochTime >= *user.CreatedTimestamp {
			// Add the user to the deletion queue
			jobs <- []string{*user.Username, *user.ID}
			counter++
		}
	}
	log.Println("[R]       : Added ", counter, " users to deletion queue out of ", strconv.Itoa(len(users)), " users searched")

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

func deleteUserWorker(id int, realmName string, clientId string, clientSecret string, targetRealm string, url string, dryRun bool, loginAsAdmin bool, jobs <-chan []string, results chan<- string, wg *sync.WaitGroup) {

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[D] panic : ", r.(string))
		}
	}()

	defer wg.Done()

	successCounter := 0
	log.Println("[D][", id, "]  : Bulk User Tool Starting")

	var client *gocloak.GoCloak
	if *useLegacyKeycloak {
		// This is for older versions of Keycloak that is based on WildFly
		client = gocloak.NewClient(url, gocloak.SetLegacyWildFlySupport())
	} else {
		// This is for newer versions of Keycloak, that is based on quarkus
		client = gocloak.NewClient(url)
	}
	if headerKey != nil && *headerKey != "" {
		client.RestyClient().Header.Set(*headerKey, *headerValue)
	}
	ids := strconv.Itoa(id)
	ctx := context.Background()
	log.Println("[D][", ids, "]  : logging into keycloak")
	var token *gocloak.JWT
	var err error
	if loginAsAdmin {
		log.Println("[D]       : logging into keycloak via admin")
		token, err = client.LoginAdmin(ctx, clientId, clientSecret, realmName)
	} else {
		log.Println("[D]       : logging into keycloak via client")
		token, err = client.LoginClient(ctx, clientId, clientSecret, realmName)
	}
	if err != nil {
		log.Println("[D][", ids, "] ", token)
		log.Println("[D][", ids, "] ", err)
		log.Println("[D][", ids, "] clientId=", clientId)
		fmt.Println("[D][", ids, "] clientId=["+clientId+"]")
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
			targetRealm,
			gocloak.GetUsersParams{
				Username: &channelData[0]})
		if err != nil {

			if err.Error() == "401 Unauthorized: HTTP 401 Unauthorized" {
				// if we get a 401, then we need to re-login.
				log.Println("[C][", ids, "] : refresh token attempt")
				fmt.Println("[C][", ids, "] : refresh token attempt")
				//wg.Done()
				// try to refresh the token
				newToken, err := client.RefreshToken(ctx, token.RefreshToken, clientId, clientSecret, realmName)
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

			if !dryRun {
				err := client.DeleteUser(ctx, token.AccessToken, targetRealm, userID)
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
func printCmdLineArgs() {

	outputCmdLineArgs(os.Stdout)

}

// Send the command line arguments to the log file
func logCmdLineArgs() {
	// Prepare a writer from logger. This assumes that log.Output() is your logger instance.
	loggerWriter := log.Writer()
	outputCmdLineArgs(loggerWriter)
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

	envClientId := os.Getenv(ENV_CLIENT_ID)
	if envClientId != "" {
		*clientId = envClientId
	}
	envClientSecret := os.Getenv(ENV_CLIENT_SECRET)
	if envClientSecret != "" {
		*clientSecret = envClientSecret
	}
	envClientRealm := os.Getenv(ENV_CLIENT_REALM)
	if envClientRealm != "" {
		*clientRealm = envClientRealm
	}
	envDestinationRealm := os.Getenv(ENV_DESTINATION_REALM)
	if envDestinationRealm != "" {
		*destinationRealm = envDestinationRealm
	}
	envUrl := os.Getenv(ENV_URL)
	if envUrl != "" {
		*url = envUrl
	}
	envDryRun := os.Getenv(ENV_DRY_RUN)
	if envDryRun != "" {
		*dryRun = envDryRun == "true"
	}

	envLogCmdValues := os.Getenv(ENV_LOG_CMD_VALUES)
	if envLogCmdValues != "" {
		*logCmdValues = envLogCmdValues == "true"
	}

	envLogDir := os.Getenv(ENV_LOG_DIR)
	if envLogDir != "" {
		//check if the logging location exists
		if _, err := os.Stat(envLogDir); os.IsNotExist(err) {
			log.Fatal("Error logging directory does not exist: ", err)
			panic("Error logging directory does not exist: " + ENV_LOG_DIR + err.Error())
		}
		*logDir = envLogDir
	}

	envLoginAsAdmin := os.Getenv(ENV_LOGIN_AS_ADMIN)
	if envLoginAsAdmin != "" {
		*loginAsAdmin = envLoginAsAdmin == "true"
	}

	envDeleteDate := os.Getenv(ENV_MAX_AGE_IN_DATE)
	if envDeleteDate != "" {
		*deleteDate = envDeleteDate
	}

	var err error

	envDays := os.Getenv(ENV_MAX_AGE_IN_DAYS)
	if envDays != "" {
		*maxAgeInDays, err = strconv.Atoi(envDays)
		if err != nil {
			log.Fatal("Error parsing days from env variable: ", err)
			panic("Error parsing threads from env variable: " + ENV_MAX_AGE_IN_DAYS + err.Error())
		}
	}

	envThreads := os.Getenv(ENV_THREADS)
	if envThreads != "" {
		*threads, err = strconv.Atoi(envThreads)
		if err != nil {
			log.Fatal("Error parsing threads from env variable: ", err)
			//fmt.Fatal("Error parsing threads from env variable: ", err)
			panic("Error parsing threads from env variable: " + ENV_THREADS + err.Error())
		}
	}

	envChannelBuffer := os.Getenv(ENV_CHANNEL_BUFFER)
	if envChannelBuffer != "" {
		*channelBuffer, err = strconv.Atoi(envChannelBuffer)
		if err != nil {
			log.Fatal("Error parsing channel buffer from env variable: ", err)
			panic("Error parsing channelBuffer from env variable:" + ENV_CHANNEL_BUFFER + err.Error())
		}
	}

	envPageSize := os.Getenv(ENV_PAGE_SIZE)
	if envPageSize != "" {
		*searchMax, err = strconv.Atoi(envPageSize)
		if err != nil {
			log.Fatal("Error parsing page size from env variable: ", err)
			panic("Error parsing pageSize from env variable:" + ENV_PAGE_SIZE + err.Error())
		}
	}

	envPageOffset := os.Getenv(ENV_PAGE_OFFSET)
	if envPageOffset != "" {
		*searchMin, err = strconv.Atoi(envPageOffset)
		if err != nil {
			log.Fatal("Error parsing page offset from env variable: ", err)
			panic("Error parsing pageOffset from env variable:" + ENV_PAGE_OFFSET + err.Error())
		}
	}

	envHeaderName := os.Getenv(ENV_HEADER_NAME)
	if envHeaderName != "" {
		envHeaderValue := os.Getenv(ENV_HEADER_VALUE)
		if envHeaderValue != "" {

			*headerKey = envHeaderName
			*headerValue = envHeaderValue
		}
	}

	logCmdLineArgs()

}

// Function that accepts an io.Writer to print or log
func outputCmdLineArgs(out io.Writer) {
	fmt.Fprintln(out, "[KeyCloak Delete via API Tool (Day/Date Based)]")
	fmt.Fprintln(out, "  Authentication:")
	fmt.Fprintln(out, "    clientId:", *clientId)
	fmt.Fprintln(out, "    clientSecret:", *clientSecret)
	fmt.Fprintln(out, "    clientRealm:", *clientRealm)
	fmt.Fprintln(out, "    destinationRealm:", *destinationRealm)
	fmt.Fprintln(out, "    loginAsAdmin:", *loginAsAdmin)
	fmt.Fprintln(out, "    url:", *url)
	fmt.Fprintln(out, "  Concurrency")
	fmt.Fprintln(out, "    channelBuffer:", *channelBuffer)
	fmt.Fprintln(out, "    threads:", *threads)
	fmt.Fprintln(out, "  Deletion Criteria")

	if *maxAgeInDays > EMPTY_DAYS {
		fmt.Fprintln(out, "    maxDaysInAge:", *maxAgeInDays, "[", daysToDate(*maxAgeInDays), "]")
	} else {
		fmt.Fprintln(out, "    maxDaysInAge:", "disabled")
	}
	if *deleteDate != "" {
		fmt.Fprintln(out, "    deleteDate:", *deleteDate)
	} else {
		fmt.Fprintln(out, "    deleteDate:", "Disabled")
	}
	fmt.Fprintln(out, "  Misc Config")
	fmt.Fprintln(out, "    dryRun:", *dryRun)
	fmt.Fprintln(out, "    logCmdValues:", *logCmdValues)
	fmt.Fprintln(out, "    logDir:", *logDir)
	fmt.Fprintln(out, "    searchMin:", *searchMin)
	fmt.Fprintln(out, "    searchMax:", *searchMax)
	fmt.Fprintln(out, " ")
}
