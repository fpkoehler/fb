package main

import (
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GeertJohan/go.rice"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/html"
)

// var templates = template.Must(template.ParseFiles(
// 	"ylogin.html",
// 	"yuser.html",
// 	"yselect.html",
// 	"yprofile.html",
// 	"yresult.html",
// 	"yregister.html",
// 	"yerror.html",
// 	"ypwreset.html",
// 	"yreset.html"))

var templates = template.New("")
var templateBox *rice.Box

func newTemplate(path string, _ os.FileInfo, _ error) error {
	if path == "" {
		return nil
	}
	templateString, err := templateBox.String(path)
	if err != nil {
		log.Panicf("Unable to read template: path=%s, err=%s", path, err)
	}
	log.Println("Parsing template", path)
	_, err = templates.New(path).Parse(templateString)
	if err != nil {
		log.Panicf("Unable to parse: path=%s, err=%s", path, err)
	}
	return nil
}

func errorPage(w http.ResponseWriter, format string, a ...interface{}) {
	data := struct {
		Error string
	}{
		Error: fmt.Sprintf(format, a...),
	}
	err := templates.ExecuteTemplate(w, "error.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getUserName(r *http.Request) (userName string) {
	if cookie, err := r.Cookie("session"); err == nil {
		userName = cookie.Value
	}
	return userName
}

func clearSession(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	http.SetCookie(w, cookie)
}

func registerGetHandler(w http.ResponseWriter, r *http.Request) {
	dummy := struct{}{}
	err := templates.ExecuteTemplate(w, "register.html", &dummy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func registerPostHandler(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	pass := r.FormValue("password")
	pas2 := r.FormValue("password2")
	nick := r.FormValue("nickname")

	if len(pass) == 0 {
		errorPage(w, "no password entered")
		return
	}

	if pass != pas2 {
		errorPage(w, "passwords do not agree")
		return
	}

	if len(nick) == 0 {
		errorPage(w, "no nickname entered")
		return
	}

	if len(nick) > 20 {
		errorPage(w, "nickname must be less than 20 characters")
		return
	}

	/*
	 * from  https://github.com/StefanSchroeder/Golang-Regex-Tutorial/blob/master/01-chapter3.markdown
	 *
	 * Interestingly the RFC 2822 which defines the format of
	 * email-addresses is pretty permissive. That makes it hard to come up
	 * with a simple regular expression that matches a valid email
	 * address. In most cases though your application can make some
	 * assumptions about addresses and I found this one sufficient for all
	 * practical purposes:
	 *
	 * (\w[-._\w]*\w@\w[-._\w]*\w\.\w{2,3})
	 *
	 * It must start with a character of the \w class. Then we can have
	 * any number of characters including the hyphen, the '.' and the
	 * underscore. We want the last character before the @ to be a
	 * 'regular' character again. We repeat the same pattern for the
	 * domain, only that the suffix (part behind the last dot) can be only
	 * 2 or 3 characters. This will cover most cases. If you come across
	 * an email address that does not match this regexp it has probably
	 * deliberately been setup to annoy you and you can therefore ignore
	 * it.
	 */

	/*
	 *  According to https://golang.org/pkg/regexp/syntax/
	 *
	 *  \w             word characters (== [0-9A-Za-z_])
	 *
	 */

	regex, err := regexp.Compile("[0-9A-Za-z_][-.0-9A-Za-z_]*[0-9A-Za-z_]@[0-9A-Za-z_][-.0-9A-Za-z_]*[0-9A-Za-z_][.][0-9A-Za-z_]{2,3}")
	if err == nil {
		if m := regex.MatchString(email); !m {
			errorPage(w, "%s is not an email address", email)
			return
		}
	} else {
		log.Println("regular expression error for email regexp", err.Error())
	}

	_, found := users[email]
	if found {
		errorPage(w, "email %s already registered", email)
		return
	}

	for _, u := range users {
		if nick == u.Name {
			errorPage(w, "nickname %s already used", nick)
			return
		}
	}

	/* hash the password */
	pwHash := fmt.Sprintf("%x", md5.Sum([]byte(pass)))

	users[email] = &User{Email: email, Name: nick, PwHash: pwHash, UserWeeks: make([]UserWeek, len(season.Week))}
	for i, _ := range users[email].UserWeeks {
		users[email].UserWeeks[i].Num = i + 1
	}
	log.Println("login: created user", email, nick, pwHash)

	cookie := &http.Cookie{
		Name:  "session",
		Value: email,
		Path:  "/",
	}
	http.SetCookie(w, cookie)

	writeUserFile(users[email])

	http.Redirect(w, r, "/user", http.StatusFound)
}

/* Password Reset
 * The request should look something like:
 * https://fpkoehler.dyndns.org/reset?token=Talo3mRjaGVzdITUAGOXYZwCMq7EtHfYH4ILcBgKaoWXDHTJOIlBUfcr
 */
func pwresetGetHandler(w http.ResponseWriter, r *http.Request) {

	/* Puts the name/value pairs on the URL Query into a map.
	 * We are interested in the token variable. */
	values := r.URL.Query()
	if values["token"] == nil {
		errorPage(w, "Internal error, token is missing")
		return
	}
	token := values.Get("token")
	email := Login(token, []byte(options.PwRecoverSecret))

	_, ok := users[email]
	if !ok {
		log.Println("Password get handler, account", email, "does not exist")
		errorPage(w, "Password reset failed, account %s does not exist", email)
		return
	}

	data := struct {
		Email string
		Token string
	}{
		Email: email,
		Token: token,
	}

	err := templates.ExecuteTemplate(w, "reset.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func pwresetPostHandler(w http.ResponseWriter, r *http.Request) {
	/* Puts the name/value pairs on the URL Query into a map.
	 * We are interested in the token variable. */
	values := r.URL.Query()
	if values["token"] == nil {
		errorPage(w, "Internal error, token is missing")
		return
	}
	token := values.Get("token")
	email := Login(token, []byte(options.PwRecoverSecret))

	user, ok := users[email]
	if !ok {
		log.Println("Password post handler, account", email, "does not exist")
		errorPage(w, "Password reset failed, account %s does not exist", email)
		return
	}

	pass := r.FormValue("password")
	pas2 := r.FormValue("password2")

	if len(pass) == 0 {
		errorPage(w, "no password entered")
		return
	}

	if pass != pas2 {
		errorPage(w, "passwords do not agree")
		return
	}

	/* hash the password */
	pwHash := fmt.Sprintf("%x", md5.Sum([]byte(pass)))
	user.PwHash = pwHash
	log.Println("password reset for", email)

	writeUserFile(user)

	http.Redirect(w, r, "/", http.StatusFound)
}

/* Password Reset Request */
func pwresetReqGetHandler(w http.ResponseWriter, r *http.Request) {
	dummy := struct{}{}
	err := templates.ExecuteTemplate(w, "pwreset.html", &dummy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func pwresetReqPostHandler(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")

	/*
	 * from  https://github.com/StefanSchroeder/Golang-Regex-Tutorial/blob/master/01-chapter3.markdown
	 *
	 * Interestingly the RFC 2822 which defines the format of
	 * email-addresses is pretty permissive. That makes it hard to come up
	 * with a simple regular expression that matches a valid email
	 * address. In most cases though your application can make some
	 * assumptions about addresses and I found this one sufficient for all
	 * practical purposes:
	 *
	 * (\w[-._\w]*\w@\w[-._\w]*\w\.\w{2,3})
	 *
	 * It must start with a character of the \w class. Then we can have
	 * any number of characters including the hyphen, the '.' and the
	 * underscore. We want the last character before the @ to be a
	 * 'regular' character again. We repeat the same pattern for the
	 * domain, only that the suffix (part behind the last dot) can be only
	 * 2 or 3 characters. This will cover most cases. If you come across
	 * an email address that does not match this regexp it has probably
	 * deliberately been setup to annoy you and you can therefore ignore
	 * it.
	 */

	/*
	 *  According to https://golang.org/pkg/regexp/syntax/
	 *
	 *  \w             word characters (== [0-9A-Za-z_])
	 *
	 */

	regex, err := regexp.Compile("[0-9A-Za-z_][-.0-9A-Za-z_]*[0-9A-Za-z_]@[0-9A-Za-z_][-.0-9A-Za-z_]*[0-9A-Za-z_][.][0-9A-Za-z_]{2,3}")
	if err == nil {
		if m := regex.MatchString(email); !m {
			errorPage(w, "%s is not an email address", email)
			return
		}
	} else {
		log.Println("regular expression error for email regexp", err.Error())
	}

	_, found := users[email]
	if !found {
		errorPage(w, "No registered user with email %s is registered", email)
		return
	}

	cookie := NewSinceNow(email, 24*time.Hour, []byte(options.PwRecoverSecret))

	body := "hi\nPlease click this link to reset your password\n" +
		"https://fpkoehler.dyndns.org/reset?token=" + cookie + "\n"

	sendEmail(email, "FB Confidence Pool Password Reset", body)

	errorPage(w, "email sent to %s", email)
}

func loginGetHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Standings []StandingRow
	}{
		Standings: getStandings(),
	}

	err := templates.ExecuteTemplate(w, "login.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loginPostHandler(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	pass := r.FormValue("password")
	redirectTarget := "/"

	if name == "" {
		log.Println("login: no user name specified")
		http.Redirect(w, r, redirectTarget, http.StatusFound)
	}

	/* TODO: Enforce some sanity for the user name */
	/* TODO: Error messages to HTML pages */

	if pass == "" {
		log.Println("login: no password specified")
		http.Redirect(w, r, redirectTarget, http.StatusFound)
	}

	/* hash the password */
	pwHash := fmt.Sprintf("%x", md5.Sum([]byte(pass)))
	log.Println("login attempt user:", name, " hash:", pwHash)

	user, ok := users[name]
	if !ok {
		log.Println("Account", name, "does not exist")
		errorPage(w, "Account %s does not exist, please register", name)
		return
	}

	if pwHash != user.PwHash {
		log.Println("login: password check failed, pwHash ", pwHash, "name.pwHash", user.PwHash)
		errorPage(w, "Password failed for %s", name)
		return
	}

	log.Println("login: found user", name)

	cookie := &http.Cookie{
		Name:  "session",
		Value: name,
		Path:  "/",
	}
	http.SetCookie(w, cookie)

	redirectTarget = "/user"

	http.Redirect(w, r, redirectTarget, http.StatusFound)
}

func logoutPostHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	log.Println("logout user", userName)
	clearSession(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func userGetHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	if userName == "" {
		http.Error(w, "no user logged in", http.StatusInternalServerError)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}

	type WeekRow struct {
		Num       int
		Indx      int
		StartDate string
		EndDate   string
		User      string
	}

	/* List of weeks to see results */
	results := make([]WeekRow, 0, len(season.Week))
	for i := 0; i <= iWeek; i++ {
		if i < len(season.Week) {
			f := WeekRow{
				Indx:      i,
				Num:       i + 1,
				StartDate: season.Week[i].weekStart.Format("Mon Jan _2"),
				EndDate:   season.Week[i].weekEnd.Format("Mon Jan _2"),
				User:      user.Email,
			}
			results = append(results, f)
		}
	}

	/* List of weeks to choose picks */
	picks := make([]WeekRow, 0, len(season.Week))
	for i := iWeek; i < len(season.Week); i++ {
		if i < len(season.Week) {
			f := WeekRow{
				Indx:      i,
				Num:       i + 1,
				StartDate: season.Week[i].weekStart.Format("Mon Jan _2"),
				EndDate:   season.Week[i].weekEnd.Format("Mon Jan _2"),
				User:      user.Email,
			}
			picks = append(picks, f)
		}
	}

	type UserStatRow struct {
		Description string
		Value       string
	}

	/* User Stats */
	weeksPlayed := 0
	totalPoints := 0
	for i, _ := range season.Week {
		if user.UserWeeks[i].Selections != nil {
			weeksPlayed++
		}
		totalPoints += user.UserWeeks[i].Points
	}
	userStats := make([]UserStatRow, 0)
	userStats = append(userStats, UserStatRow{"Weeks Played", strconv.Itoa(weeksPlayed)})
	userStats = append(userStats, UserStatRow{"Total Points", strconv.Itoa(totalPoints)})
	if weeksPlayed > 0 {
		aveScore := float64(totalPoints) / float64(weeksPlayed)
		userStats = append(userStats, UserStatRow{"Avg. Points", strconv.FormatFloat(aveScore, 'f', 1, 32)})
	}

	data := struct {
		Name      string
		Date      string
		Picks     []WeekRow
		Results   []WeekRow
		Standings []StandingRow
		Stats     []UserStatRow
	}{
		Name:      user.Name,
		Date:      time.Now().Format("Mon Jan _2 MST"),
		Picks:     picks,
		Results:   results,
		Standings: getStandings(),
		Stats:     userStats,
	}

	err := templates.ExecuteTemplate(w, "user.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func profileGetHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	if userName == "" {
		http.Error(w, "no user logged in", http.StatusInternalServerError)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}

	err := templates.ExecuteTemplate(w, "profile.html", user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func resultGetHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	if userName == "" {
		http.Error(w, "no user logged in", http.StatusInternalServerError)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}

	/* path will look something like /results/fred/1
	 * where fred is the player and 1 is the week index.
	 * Note that for this form, we might be showing the
	 * results for another user/player.  The userName is
	 * the user logged in, player will be who we want to show
	 * the results for */
	f := func(c rune) bool { return c == '/' }
	fields := strings.FieldsFunc(html.EscapeString(r.URL.Path), f)

	if len(fields) != 3 {
		log.Println("bad result URL", r.URL.Path, "expected 3 fields")
		http.Error(w, "bad result URL "+r.URL.Path+" expected 3 fields", http.StatusInternalServerError)
	}

	player, ok := users[fields[1]]
	if !ok {
		log.Println("no player for", fields[1], "URL:", r.URL.Path)
		http.Error(w, "no player for "+fields[1], http.StatusInternalServerError)
		return
	}

	iw, err := strconv.Atoi(fields[2])
	if err != nil {
		log.Println("user", user, "result week", r.URL.Path, "does not exist")
		http.Error(w, "user "+userName+" "+r.URL.Path+" does not exist", http.StatusInternalServerError)
		return
	}

	/* Build data for the results table */
	type ResultsRow struct {
		Time       string
		TeamV      string
		TeamH      string
		Pick       string
		Confidence int
		Winner     string
		Points     int
	}

	finished := make([]ResultsRow, 0)
	inProgress := make([]ResultsRow, 0)
	future := make([]ResultsRow, 0)

	for _, s := range player.UserWeeks[iw].Selections {
		var game Game
		found := false
		for _, game = range season.Week[iw].Games {
			if s.Team == game.TeamH || s.Team == game.TeamV {
				found = true
				break
			}
		}

		if !found {
			log.Println("Results game not found for weekIndx", iw, "selection", s)
			continue
		}

		points := 0
		winner := "tie"
		time := game.Time

		if game.Status == Future {
			time = game.Day.AddDayTime(game.Time).Format("Mon Jan _2 3:04:05PM MST")
		}

		if game.Status == InProgress || game.Status == Finished {
			/* get current score */
			scoreV, err := strconv.Atoi(game.ScoreV)
			if err != nil {
				continue
			}
			scoreH, err := strconv.Atoi(game.ScoreH)
			if err != nil {
				continue
			}

			if scoreH > scoreV {
				winner = game.TeamH
			}

			if scoreH < scoreV {
				winner = game.TeamV
			}

			if s.Team == winner {
				points = s.Confidence
			}
		}

		resultsRow := ResultsRow{
			Time:       time,
			TeamV:      game.TeamV,
			TeamH:      game.TeamH,
			Pick:       s.Team,
			Confidence: s.Confidence,
			Winner:     winner,
			Points:     points,
		}

		switch game.Status {
		case Future:
			future = append(future, resultsRow)
		case InProgress:
			inProgress = append(inProgress, resultsRow)
		case Finished:
			finished = append(finished, resultsRow)
		}
	}

	/* Build data for the players table */
	type PlayerRow struct {
		URL    string
		User   string
		Points int
	}

	players := make([]PlayerRow, 0)
	for _, u := range users {
		playerRow := PlayerRow{
			URL:    fmt.Sprintf("/results/%s/%d", u.Email, iw),
			User:   u.Name,
			Points: u.UserWeeks[iw].Points,
		}
		players = append(players, playerRow)
	}

	data := struct {
		User       string
		UWeek      int
		IWeek      int
		Points     int
		Player     string
		Finished   []ResultsRow
		InProgress []ResultsRow
		Future     []ResultsRow
		Players    []PlayerRow
	}{
		User:       user.Name,
		UWeek:      season.Week[iw].Num,
		IWeek:      iw,
		Points:     player.UserWeeks[iw].Points,
		Player:     player.Name,
		Finished:   finished,
		InProgress: inProgress,
		Future:     future,
		Players:    players,
	}

	err = templates.ExecuteTemplate(w, "result.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type UserGameTmpl struct {
	Num        int
	TeamV      string
	TeamH      string
	CheckedV   string
	CheckedH   string
	TeamSel    string
	ScoreV     string
	ScoreH     string
	CSS        string
	Confidence int
	When       string
	Status     string
}

func selectGetHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	if userName == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}
	/* path will look something like /select/1
	 * Extract the number */
	week, err := strconv.Atoi(strings.Trim(r.URL.Path, "/select/"))
	if err != nil {
		log.Println("user", user, "selected", r.URL.Path, "does not exist")
		http.Error(w, "user "+userName+" "+r.URL.Path+" does not exist", http.StatusInternalServerError)
		return
	}
	if user.UserWeeks[week].Selections == nil {
		log.Println("selectGetHandler for user", user.Email, "no selections for week", week)
	} else {
		log.Println("selectGetHandler for user", user.Email, "#selections for week", week, len(user.UserWeeks[week].Selections))
	}

	/* create an anonymous struct to pass to ExecuteTemplate */
	/* http://julianyap.com/2013/09/23/using-anonymous-structs-to-pass-data-to-templates-in-golang.html */
	data := struct {
		User     string
		Week     int
		UWeek    int
		Points   int // TODO: is this being used?
		NumGames int
		Games    []UserGameTmpl
		Started  []UserGameTmpl
	}{
		User:     user.Name,
		Week:     week,
		UWeek:    week + 1,
		Points:   user.UserWeeks[week].Points,
		NumGames: len(season.Week[week].Games),
	}

	data.Games = make([]UserGameTmpl, 0, len(season.Week[week].Games))
	data.Started = make([]UserGameTmpl, 0, len(season.Week[week].Games))
	for indx, game := range season.Week[week].Games {
		confidence := indx + 1
		checkV := "checked"
		teamSel := game.TeamV
		checkH := ""
		when := ""
		css := "floating"
		status := game.Time

		if game.Status == Future {
			status = game.Day.AddDayTime(game.Time).Format("Mon Jan _2 3:04pm MST")
		}

		/* see if the user already made a selection for this game */
		var pSelection *Selection
		pSelection = nil
		for is, s := range user.UserWeeks[week].Selections {
			if s.Team == game.TeamV || s.Team == game.TeamH {
				pSelection = &user.UserWeeks[week].Selections[is]
				break
			}
		}

		if pSelection == nil {
			if game.Status == Future {
				confidence = indx + 1
			} else {
				confidence = 0
				teamSel = "--"
			}
		} else {
			confidence = pSelection.Confidence

			/* Convert the time string in the selection to a "time", then format it.
			 * The first parameter to the Parse method is from https://golang.org/pkg/time/#Time.String */
			t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", pSelection.When)
			if err != nil {
				log.Println("user", user.Email, "week", week, "selection", indx,
					"timeStr", pSelection.When, ":", err.Error())
			} else {
				when = t.Format("Mon Jan _2 3:04:05PM MST 2006")
			}

			if pSelection.Team == game.TeamH {
				checkV = ""
				checkH = "checked"
				teamSel = game.TeamH
			}
		}

		u := UserGameTmpl{
			Num:        indx + 1,
			TeamV:      game.TeamV,
			TeamH:      game.TeamH,
			ScoreV:     game.ScoreV,
			ScoreH:     game.ScoreH,
			CheckedV:   checkV,
			CheckedH:   checkH,
			TeamSel:    teamSel,
			Confidence: confidence,
			When:       when,
			Status:     status,
			CSS:        css, // not used anymore
		}

		if game.Status == Future {
			data.Games = append(data.Games, u)
		} else {
			data.Started = append(data.Started, u)
		}
	}

	err = templates.ExecuteTemplate(w, "select.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func selectDnDGetHandler(w http.ResponseWriter, r *http.Request) {
	userName := getUserName(r)
	if userName == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}
	/* path will look something like /selectDnD/1
	 * Extract the number */
	week, err := strconv.Atoi(strings.Trim(r.URL.Path, "/selectDnD/"))
	if err != nil {
		log.Println("user", user, "selected", r.URL.Path, "does not exist")
		http.Error(w, "user "+userName+" "+r.URL.Path+" does not exist", http.StatusInternalServerError)
		return
	}
	if user.UserWeeks[week].Selections == nil {
		log.Println("selectGetHandler for user", user.Email, "no selections for week", week)
	} else {
		log.Println("selectGetHandler for user", user.Email, "#selections for week", week, len(user.UserWeeks[week].Selections))
	}

	/* create an anonymous struct to pass to ExecuteTemplate */
	/* http://julianyap.com/2013/09/23/using-anonymous-structs-to-pass-data-to-templates-in-golang.html */
	data := struct {
		User     string
		Week     int
		UWeek    int
		Points   int // TODO: is this being used?
		NumGames int
		Games    []UserGameTmpl
		Started  []UserGameTmpl
	}{
		User:     user.Name,
		Week:     week,
		UWeek:    week + 1,
		Points:   user.UserWeeks[week].Points,
		NumGames: len(season.Week[week].Games),
	}

	data.Games = make([]UserGameTmpl, 0, len(season.Week[week].Games))
	data.Started = make([]UserGameTmpl, 0, len(season.Week[week].Games))
	for indx, game := range season.Week[week].Games {
		confidence := indx + 1
		checkV := "checked"
		teamSel := game.TeamV
		checkH := ""
		when := ""
		css := "floating"
		status := game.Time

		if game.Status == Future {
			status = game.Day.AddDayTime(game.Time).Format("Mon Jan _2 3:04pm MST")
		}

		/* see if the user already made a selection for this game */
		var pSelection *Selection
		pSelection = nil
		for is, s := range user.UserWeeks[week].Selections {
			if s.Team == game.TeamV || s.Team == game.TeamH {
				pSelection = &user.UserWeeks[week].Selections[is]
				break
			}
		}

		if pSelection == nil {
			if game.Status == Future {
				confidence = indx + 1
			} else {
				confidence = 0
				teamSel = "--"
			}
		} else {
			confidence = pSelection.Confidence

			/* Convert the time string in the selection to a "time", then format it.
			 * The first parameter to the Parse method is from https://golang.org/pkg/time/#Time.String */
			t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", pSelection.When)
			if err != nil {
				log.Println("user", user.Email, "week", week, "selection", indx,
					"timeStr", pSelection.When, ":", err.Error())
			} else {
				when = t.Format("Mon Jan _2 3:04:05PM MST 2006")
			}

			if pSelection.Team == game.TeamH {
				checkV = ""
				checkH = "checked"
				teamSel = game.TeamH
			}
		}

		u := UserGameTmpl{
			Num:        indx + 1,
			TeamV:      game.TeamV,
			TeamH:      game.TeamH,
			ScoreV:     game.ScoreV,
			ScoreH:     game.ScoreH,
			CheckedV:   checkV,
			CheckedH:   checkH,
			TeamSel:    teamSel,
			Confidence: confidence,
			When:       when,
			Status:     status,
			CSS:        css, // not used anymore
		}

		if game.Status == Future {
			data.Games = append(data.Games, u)
		} else {
			data.Started = append(data.Started, u)
		}
	}

	err = templates.ExecuteTemplate(w, "selectDnD.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func selectPostHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("selectPost URL", r.URL.Path)

	userName := getUserName(r)
	if userName == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	user, ok := users[userName]
	if !ok {
		http.Error(w, "no user for "+userName, http.StatusInternalServerError)
		return
	}
	log.Println("selectPostHandler for user", user.Email)

	/* path will look something like /save/1
	 * Extract the number */
	week, err := strconv.Atoi(strings.Trim(r.URL.Path, "/save/"))
	if err != nil {
		log.Println("user", user, "selected", r.URL.Path, "does not exist")
		http.Error(w, "user "+userName+" "+r.URL.Path+" does not exist", http.StatusInternalServerError)
		return
	}
	log.Println("User saving week", week)

	if user.UserWeeks[week].Selections == nil {
		user.UserWeeks[week].Selections = make([]Selection, 0, 16)
	}

	// var pGame *Game
	var pSelection *Selection

	when := time.Now()
	for _, game := range season.Week[week].Games {
		if game.Status == InProgress || game.Status == Finished {
			continue
		}

		if game.Status == Future {
			gameTime := game.Day.AddDayTime(game.Time)
			if when.After(gameTime) {
				/* We have not updated the game status yet,
				 * but we are past the start of the game.  */
				log.Println("user", user.Name, "ignoring selection", game.TeamV, game.TeamH, "it started", gameTime)
				continue
			}
		}

		whoWins := r.FormValue(game.TeamV)
		if whoWins == "" {
			/* game not on form because it already started */
			continue
		}

		if strings.Compare(whoWins, "home") == 0 {
			whoWins = game.TeamH
		} else {
			whoWins = game.TeamV
		}

		/* confidence value */
		confidenceStr := r.FormValue("confidence" + game.TeamV)
		confidence, err := strconv.Atoi(confidenceStr)
		if err != nil {
			log.Println("Error: user", user, "selection", whoWins, "bad confidence value:", confidenceStr)
			confidence = 16
		}

		/* see if the user already made a selection for this game */
		pSelection = nil
		for is, s := range user.UserWeeks[week].Selections {
			if s.Team == game.TeamV || s.Team == game.TeamH {
				pSelection = &user.UserWeeks[week].Selections[is]
				break
			}
		}

		if pSelection == nil {
			/* no previous selection, append to user's selections */
			selection := Selection{Team: whoWins, Confidence: confidence, When: when.String()}
			user.UserWeeks[week].Selections = append(user.UserWeeks[week].Selections, selection)
		} else {
			/* has the selection changed? If it hasn't, do nothing */
			if pSelection.Team != whoWins || pSelection.Confidence != confidence {
				pSelection.Team = whoWins
				pSelection.Confidence = confidence
				pSelection.When = when.String()
			}
		}
	}

	/* Make sure confidence values are not repeated */
	//	validArray := make([]string, len(user.UserWeeks[week].Selections)+1)
	validArray := make([]string, len(season.Week[week].Games)+1)
	log.Println(user.UserWeeks[week].Selections)
	for _, s := range user.UserWeeks[week].Selections {
		if validArray[s.Confidence] == "" {
			validArray[s.Confidence] = s.Team
		} else {
			errorPage(w, "Can not reuse confidences, you have both %s and %s with a confidence of %d\n",
				s.Team, validArray[s.Confidence], s.Confidence)
			// user.UserWeeks[week].Selections[i].Confidence = 0
			return
		}
	}

	writeUserFile(user)

	/* back to main user page */
	http.Redirect(w, r, "/user", http.StatusFound)
}

func webSrv() {
	// Load and parse templates (from binary or disk)
	templateBox = rice.MustFindBox("templates")
	templateBox.Walk("", newTemplate)

	// The resources directory that contains CSS and JavaScript files
	resourceBox := rice.MustFindBox("resources")
	resourceFileServer := http.StripPrefix("/resources/", http.FileServer(resourceBox.HTTPBox()))
	http.Handle("/resources/", resourceFileServer)

	/* handlers for GETs */
	http.HandleFunc("/", loginGetHandler)
	http.HandleFunc("/user", userGetHandler)
	http.HandleFunc("/profile", profileGetHandler)
	http.HandleFunc("/select/", selectGetHandler)
	http.HandleFunc("/selectDnD/", selectDnDGetHandler)
	http.HandleFunc("/results/", resultGetHandler)
	http.HandleFunc("/register", registerGetHandler)
	http.HandleFunc("/pwreset", pwresetReqGetHandler)
	http.HandleFunc("/reset", pwresetGetHandler)

	/* handlers for POSTs */
	http.HandleFunc("/login", loginPostHandler)
	http.HandleFunc("/logout", logoutPostHandler)
	http.HandleFunc("/save/", selectPostHandler)
	http.HandleFunc("/Register", registerPostHandler)
	http.HandleFunc("/PwReset", pwresetReqPostHandler)
	http.HandleFunc("/Reset", pwresetPostHandler)

	/* put css files in the resources directory
	 * See http://stackoverflow.com/questions/13302020/rendering-css-in-a-go-web-application
	 * and https://groups.google.com/forum/#!topic/golang-nuts/bStLPdIVM6w
	 * for hiding contents of that directory  */
	//	http.Handle("/resources/", http.StripPrefix("/resources/", http.FileServer(http.Dir("resources"))))

	// Start the HTTPS server in a goroutine
	go func() {
		/* Using Let's Encrypt (https://letsencrypt.org/)
		 * This is supported in Go's experimental autocert package */

		m := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(options.HostWhiteList),
			Cache:      autocert.DirCache("./certCache"),
			Email:      options.AdminEmail,
		}

		/* When we are not in the HostWhitelist, we want to defer to the
		 * self-signed certs server.{pem,key}.  If GetCertificate returns nil,
		 * then the certificate specified in ListenAndServeTLS() will be used
		 * (assuming tls.Config.NameToCertificate is nil).
		 *
		 * So given all of that, we have this wrapper function which calls
		 * the autocert manager's GetCertificate and if that errors, we
		 * return nil, thus allowing http.TLS to use the specified self-
		 * signed certificates */
		getCert := func(hello *tls.ClientHelloInfo) (cert *tls.Certificate, err error) {
			cert, err = m.GetCertificate(hello)
			if err != nil {
				return nil, nil
			}
			return cert, err
		}

		s := &http.Server{
			//Addr:      ":https",
			Addr:      ":4430",
			TLSConfig: &tls.Config{GetCertificate: getCert},
		}

		err := s.ListenAndServeTLS("server.pem", "server.key")
		if err != nil {
			log.Fatalf("ListenAndServeTLS error: %v", err)
		}
	}()

	log.Println("Starting Web Server")

	// Start the HTTP server and redirect all incoming connections to HTTPS
	err := http.ListenAndServe(":8080", http.HandlerFunc(redirectToHttps))
	if err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
