package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	UpdateFromWeb   bool
	ScheduleFromWeb bool
	RedirectUrl     string
	ScheduleUrl     string
	UpdateUrl       string
	PwRecoverSecret string
	AdminEmail      string
	AdminEmailPw    string
}

type GameStatus int

const (
	Future GameStatus = iota
	InProgress
	Finished
)

type Game struct {
	TeamV  string
	TeamH  string
	ScoreV string
	ScoreH string
	Time   string
	Day    Date
	Status GameStatus
}

type Week struct {
	Num        int
	weekStart  time.Time
	weekEnd    time.Time
	Games      []Game
	teamToGame map[string]*Game
}

type Season struct {
	Year int
	Week [17]Week
}

type Selection struct {
	Team       string
	Confidence int
	When       string // time the selection was made
}

type UserWeek struct {
	Num        int `xml:"Week,attr"`
	Points     int
	Selections []Selection
}

type User struct {
	Email     string
	Name      string
	PwHash    string
	Subscribe bool
	UserWeeks []UserWeek
	fileLock  sync.Mutex
}

type StandingRow struct {
	Name        string
	Total       int
	WeeksPlayed int
	WeeksWon    int
	AvePerWeek  string
}

/* For sorting the standings, note we want
 * ascending order, so "Less" actually returns greater */
type ByStandingRow []StandingRow

func (a ByStandingRow) Len() int           { return len(a) }
func (a ByStandingRow) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByStandingRow) Less(i, j int) bool { return a[i].Total > a[j].Total }

/**********************************************************/

var options Options

var season = Season{Year: 2017}

// will be indexed by user name
var users map[string]*User

var toTeam map[string]string

/* Current index into season.Week[] */
var iWeek int

var seasonEnded bool

/**********************************************************/

func init() {
	toTeam = map[string]string{
		"Arizona":              "Cardinals",
		"Arizona Cardinals":    "Cardinals",
		"Atlanta":              "Falcons",
		"Atlanta Falcons":      "Falcons",
		"Baltimore":            "Ravens",
		"Baltimore Ravens":     "Ravens",
		"Buffalo":              "Bills",
		"Buffalo Bills":        "Bills",
		"Carolina":             "Panthers",
		"Carolina Panthers":    "Panthers",
		"Chicago":              "Bears",
		"Chicago Bears":        "Bears",
		"Cincinnati":           "Bengals",
		"Cincinnati Bengals":   "Bengals",
		"Cleveland":            "Browns",
		"Cleveland Browns":     "Browns",
		"Dallas":               "Cowboys",
		"Dallas Cowboys":       "Cowboys",
		"Denver":               "Broncos",
		"Denver Broncos":       "Broncos",
		"Detroit":              "Lions",
		"Detroit Lions":        "Lions",
		"Green Bay":            "Packers",
		"Green Bay Packers":    "Packers",
		"Houston":              "Texans",
		"Houston Texans":       "Texans",
		"Indianapolis":         "Colts",
		"Indianapolis Colts":   "Colts",
		"Jacksonville":         "Jaguars",
		"Jacksonville Jaguars": "Jaguars",
		"Kansas City":          "Chiefs",
		"Kansas City Chiefs":   "Chiefs",
		"Los Angeles":          "Rams",
		"Los Angeles Rams":     "Rams",
		"Los Angeles Chargers": "Chargers",
		"Miami":                "Dolphins",
		"Miami Dolphins":       "Dolphins",
		"Minnesota":            "Vikings",
		"Minnesota Vikings":    "Vikings",
		"New England":          "Patriots",
		"New England Patriots": "Patriots",
		"New Orleans":          "Saints",
		"New Orleans Saints":   "Saints",
		"NY Giants":            "Giants",
		"New York Giants":      "Giants",
		"NY Jets":              "Jets",
		"New York Jets":        "Jets",
		"Oakland":              "Raiders",
		"Oakland Raiders":      "Raiders",
		"Philadelphia":         "Eagles",
		"Philadelphia Eagles":  "Eagles",
		"Pittsburgh":           "Steelers",
		"Pittsburgh Steelers":  "Steelers",
		"San Diego":            "Chargers",
		"San Diego Chargers":   "Chargers",
		"San Francisco":        "49ers",
		"San Francisco 49ers":  "49ers",
		"Seattle":              "Seahawks",
		"Seattle Seahawks":     "Seahawks",
		"Tampa Bay":            "Buccaneers",
		"Tampa Bay Buccaneers": "Buccaneers",
		"Tennessee":            "Titans",
		"Tennessee Titans":     "Titans",
		"Washington":           "Redskins",
		"Washington Redskins":  "Redskins",
	}
}

/**********************************************************/

type GameStartError struct {
	What string
}

func (e GameStartError) Error() string {
	//	return fmt.Sprintf("%v", e.What)
	return e.What
}

/**********************************************************/

func updateWeekIndex() {
	/* where are we in the schedule?
	 * Update global variable iWeek */
	var t time.Time

	iWeek = -1

	t = time.Now()
	log.Println("What week are we in? current time:", t)
	for i, _ := range season.Week {
		/* endWeek is the start time of the last game,
		 * so consider the end of the week 24 hours later */
		weekEndTime := season.Week[i].weekEnd.Add(24 * time.Hour)
		log.Println("week", season.Week[i].Num, "start", season.Week[i].weekStart, "end+24", weekEndTime)
		if t.After(season.Week[i].weekStart) && t.Before(weekEndTime) {
			log.Println("In week", season.Week[i].Num)
			iWeek = i
			break
		}
		if t.Before(season.Week[i].weekStart) {
			log.Println("Before week", season.Week[i].Num)
			iWeek = i
			break
		}
	}

	/* are we at the end of the season? */
	weekEndTime := season.Week[len(season.Week)-1].weekEnd.Add(24 * time.Hour)
	if t.After(weekEndTime) {
		iWeek = len(season.Week) - 1
		seasonEnded = true
	}

	if iWeek == -1 {
		log.Println("Could not calculate iWeek")
		panic("Could not calculate iWeek")
	}
}

/**********************************************************/

func getStandings() []StandingRow {
	/* note that we are not going up to the current week,
	 * unless the seaon is over */
	lastIWeek := iWeek
	if seasonEnded {
		lastIWeek++
	}

	highScore := make([]int, lastIWeek)
	standings := make([]StandingRow, 0)

	for _, u := range users {
		for i := 0; i < lastIWeek; i++ {
			if u.UserWeeks[i].Points > highScore[i] {
				highScore[i] = u.UserWeeks[i].Points
			}
		}
	}

	for _, u := range users {
		weeksWon := 0
		weeksPlayed := 0
		totalForUser := 0
		aveScoreStr := "0.0"

		for i := 0; i < lastIWeek; i++ {
			totalForUser += u.UserWeeks[i].Points
			if u.UserWeeks[i].Points == highScore[i] {
				weeksWon++
			}
			if u.UserWeeks[i].Selections != nil {
				weeksPlayed++
			}
		}

		if weeksPlayed > 0 {
			aveScore := float64(totalForUser) / float64(weeksPlayed)
			aveScoreStr = strconv.FormatFloat(aveScore, 'f', 1, 32)
		}

		x := StandingRow{Name: u.Name, Total: totalForUser, WeeksPlayed: weeksPlayed, WeeksWon: weeksWon, AvePerWeek: aveScoreStr}
		standings = append(standings, x)
	}

	sort.Sort(ByStandingRow(standings))

	return standings
}

/**********************************************************/

/* if the dayString is before the start of iWeek then return true */
func beforeIWeek(date Date) bool {
	t := date.Time()
	t = t.Add(6 * time.Hour) // so that the day in California is the same as New York

	iWeekDay := season.Week[iWeek].weekStart.YearDay()
	day := t.YearDay()

	if t.Year() < season.Week[iWeek].weekStart.Year() {
		log.Println("beforeIWeek returning true, year", t.Year(), "<", season.Week[iWeek].weekStart.Year())
		return true
	}

	if day < iWeekDay {
		log.Println("beforeIWeek returning true, day", day, "<", iWeekDay)
		return true
	}

	return false
}

/**********************************************************/

func updateGames() {
	for {
		fmt.Println("updating games for week indx", iWeek, " @", time.Now())

		var p ParseHTML
		var dayStr string
		var teamHStr string
		var teamVStr string
		var scoreVStr string
		var scoreHStr string
		var stateStr string

		gamesInProgress := false
		gameTimes := make(map[int64]bool)

		if options.UpdateFromWeb {
			log.Println("Updating games for week indx", iWeek, "from", options.UpdateUrl, ":")
			resp, err := http.Get(options.UpdateUrl)
			for err != nil {
				log.Println("Error from http.Get, retry in 1 minute:", err.Error())
				time.Sleep(1 * time.Minute)
				resp, err = http.Get(options.UpdateUrl)
			}
			defer resp.Body.Close() //TODO: should this be closed sooner?
			p.Init(resp.Body)
		} else {
			fileName := options.UpdateUrl
			log.Println("Updating games for week indx", iWeek, "from", fileName, ":")
			file, err := os.Open(fileName) // For read access.
			defer file.Close()
			if err != nil {
				log.Println("cannot open ", fileName)
				return
			}
			p.Init(file)
		}

		i := 0
		for p.SeekTag("sblivegame") {

			log.Println("-----------------")
			p.SeekTag("twofifths fleft")
			dayStr = p.GetText()
			dayStr = fmt.Sprintf("%s/%d", dayStr, time.Now().Year())

			var date Date
			date.Set(dayStr)

			if beforeIWeek(date) {
				log.Println("Week mismatch, dayStr", dayStr, "iWeek start", season.Week[iWeek].weekStart)
				break
			}

			p.SeekTag("twofifths fleft right")
			stateStr = p.GetText()

			var gameStatus GameStatus
			switch {
			case strings.Contains(stateStr, "FINAL") || strings.Contains(stateStr, "F/OT"):
				gameStatus = Finished
			case strings.Contains(stateStr, "AM") || strings.Contains(stateStr, "PM"):
				gameStatus = Future
			default:
				gameStatus = InProgress
			}

			p.SeekTag("teamcol")
			teamVStr = toTeam[p.GetText()]
			p.SeekTag("scorecol")
			scoreVStr = p.GetText()
			log.Println(teamVStr, " ", scoreVStr)

			p.SeekTag("teamcol")
			teamHStr = toTeam[p.GetText()]
			p.SeekTag("scorecol")
			scoreHStr = p.GetText()
			log.Println(teamHStr, " ", scoreHStr)
			log.Println(stateStr)

			game := Game{
				TeamV:  teamVStr,
				TeamH:  teamHStr,
				Time:   stateStr,
				Day:    date,
				Status: gameStatus,
				ScoreV: scoreVStr,
				ScoreH: scoreHStr,
			}

			switch gameStatus {
			case Future:
				t := date.AddDayTime(stateStr)
				fmt.Println("game starts", t.Format(time.UnixDate), "from now", t.Sub(time.Now()))
				log.Println("game starts", t.Format(time.UnixDate), "from now", t.Sub(time.Now()))
				/* store start time in a map. This is a convenient way to
				 * keep track of start times for the week without duplicates */
				gameTimes[t.Unix()] = true
			case InProgress:
				gamesInProgress = true
			case Finished:
			}

			// TODO: need to make sure we update the correct game
			season.Week[iWeek].Games[i] = game

			i++
		}

		/* if any game has started, for every user, compute score */
		for _, u := range users {
			log.Println("---User", u.Email, "---")
			totalPoints := 0
			for _, s := range u.UserWeeks[iWeek].Selections {
				var game Game
				found := false
				for _, game = range season.Week[iWeek].Games {
					if s.Team == game.TeamH || s.Team == game.TeamV {
						found = true
						break
					}
				}

				if !found {
					_, _, line, _ := runtime.Caller(0)
					fmt.Println("line", line, "Could not find game for user", u.Email, "selection", s.Team)
					log.Println("line", line, "Could not find game for user", u.Email, "selection", s.Team)
					continue
				}

				if game.Status == InProgress || game.Status == Finished {
					/* the game has started or finished */

					/* get current score */
					scoreV, err := strconv.Atoi(game.ScoreV)
					if err != nil {
						continue
					}
					scoreH, err := strconv.Atoi(game.ScoreH)
					if err != nil {
						continue
					}

					/* team1 verb team1 user %d points */
					points := 0
					verb := "even with"
					team1 := game.TeamH
					team2 := game.TeamV
					if scoreH != scoreV {
						verb = "leads"
					}
					if scoreH > scoreV {
						if s.Team == game.TeamH {
							points = s.Confidence
						}
					}
					if scoreH < scoreV {
						team1 = game.TeamV
						team2 = game.TeamH
						if s.Team == game.TeamV {
							points = s.Confidence
						}
					}

					totalPoints += points

					log.Printf("\t%s %s %s user %s %d points\n", team1, verb, team2, u.Email, points)
				}
			}

			if u.UserWeeks[iWeek].Points == totalPoints {
				log.Printf("user %s weekIndx %d totalPoints %d unchanged\n", u.Email, iWeek, totalPoints)
			} else {
				u.UserWeeks[iWeek].Points = totalPoints
				log.Printf("user %s weekIndx %d totalPoints %d\n", u.Email, iWeek, totalPoints)
				writeUserFile(u)
			}
		}

		/* Compute the next time to loop */

		/* Default is 8am the next day */
		nextTime := time.Now().AddDate(0, 0, 1) // add one day
		y, m, d := nextTime.Date()
		loc, _ := time.LoadLocation("America/Los_Angeles")
		nextTime = time.Date(y, m, d, 8, 0, 0, 0, loc)

		/* ... when is the next game time from now */
		nowUnix := time.Now().Unix()
		for k := range gameTimes {
			if k < nowUnix {
				/* The game has already started but the web
				 * site might not have updated to reflect that yet.
				 * This usually happens within the first few
				 * minutes of a game.  So if k (the gameTime)
				 * is within 30 minutes of now, set k=now+30m
				 * so that we check back. */
				if nowUnix-k < (30 * 60) {
					/* the game already started, check back 30m from now */
					k = nowUnix + (30 * 60)
				}
			}

			if k > nowUnix && k < nextTime.Unix() {
				nextTime = time.Unix(k, 0)
			}
		}

		/* ... if games in progress, then in a few minutes */
		if gamesInProgress {
			nextTime = time.Now().Add(5 * time.Minute)
			log.Println("Games are in progress")
		}

		sleep := nextTime.Sub(time.Now())
		log.Println("next time to update", nextTime, "sleep", sleep)

		time.Sleep(sleep)

		updateWeekIndex()
	}
}

/*****************************************************************************/

func updateUserScoresWeekIndex(iw int) {
	log.Println("Updating user scores for week indx", iw)

	for _, u := range users {
		log.Println("---User", u.Email, "---")
		totalPoints := 0
		for _, s := range u.UserWeeks[iw].Selections {
			game := season.Week[iw].teamToGame[s.Team]
			if game == nil {
				_, _, line, _ := runtime.Caller(0)
				fmt.Println("line", line, "Could not find game for user", u.Email, "selection", s.Team)
				log.Println("line", line, "Could not find game for user", u.Email, "selection", s.Team)
				continue
			}
			if game.Status == InProgress || game.Status == Finished {
				/* the game has started or finished */

				/* get current score */
				scoreV, err := strconv.Atoi(game.ScoreV)
				if err != nil {
					continue
				}
				scoreH, err := strconv.Atoi(game.ScoreH)
				if err != nil {
					continue
				}

				/* team1 verb team1 user %d points */
				points := 0
				verb := "even with"
				team1 := game.TeamH
				team2 := game.TeamV
				if scoreH != scoreV {
					verb = "leads"
				}
				if scoreH > scoreV {
					if s.Team == game.TeamH {
						points = s.Confidence
					}
				}
				if scoreH < scoreV {
					team1 = game.TeamV
					team2 = game.TeamH
					if s.Team == game.TeamV {
						points = s.Confidence
					}
				}

				totalPoints += points

				log.Printf("\t%s %s %s user %s %d points\n", team1, verb, team2, u.Email, points)
			}
		}

		if u.UserWeeks[iw].Points == totalPoints {
			log.Printf("user %s weekIndx %d totalPoints %d unchanged\n", u.Email, iw, totalPoints)
		} else {
			u.UserWeeks[iw].Points = totalPoints
			log.Printf("user %s weekIndx %d totalPoints %d\n", u.Email, iw, totalPoints)
			writeUserFile(u)
		}
	}
}

/*****************************************************************************/

func updateUserScores() {
	for iw, _ := range season.Week {
		updateUserScoresWeekIndex(iw)
	}
}

/*****************************************************************************/

// ByInt64 implements sort.Interface for []int64.
type ByInt64 []int64

func (a ByInt64) Len() int           { return len(a) }
func (a ByInt64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByInt64) Less(i, j int) bool { return a[i] < a[j] }

/*****************************************************************************/

func getSchedule(week int, url string) {
	var p ParseHTML
	var dayStr string
	var timeStr string
	var teamHStr string
	var teamVStr string
	var scoreVStr string
	var scoreHStr string
	var b bool
	//	var stateStr string

	allGamesFinal := true
	gamesInProgress := false
	gameTimes := make(map[int64]bool)

	season.Week[week].Games = make([]Game, 0, 16)
	season.Week[week].teamToGame = make(map[string]*Game)

	log.Println("Getting games for week indx", week, "from", url, ":")

	if options.ScheduleFromWeb {
		resp, err := http.Get(url)
		if err != nil {
			log.Println("Error from http.Get:", err.Error())
			return
		}
		defer resp.Body.Close()
		p.Init(resp.Body)
	} else {
		fileName := url
		file, err := os.Open(fileName) // For read access.
		if err != nil {
			log.Println("cannot open ", fileName)
			return
		}
		// TODO: defer closing file
		p.Init(file)
	}

	for p.SeekTag("divider", "left") {

		if strings.Compare("divider", p.Tok.Attr[0].Val) == 0 {
			dayStr = p.GetText()

			p.SeekTag("left") // advances to game status/time
		}

		scoreVStr = ""
		scoreHStr = ""

		timeStr = p.GetText()

		p.SeekTag("left")
		teamVStr = toTeam[p.GetText()]

		if strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT") {
			scoreVStr, b = p.SeekBoldText()
			if !b {
				fmt.Println("SeekBoldText() failed after teamVStr", teamVStr)
				return
			}
		}

		p.SeekTag("left")
		teamHStr = toTeam[p.GetText()]

		if strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT") {
			scoreHStr, b = p.SeekBoldText()
			if !b {
				fmt.Println("SeekBoldText() failed after teamHStr", teamHStr)
				return
			}
		}

		log.Println(teamVStr, scoreVStr, "vs", teamHStr, scoreHStr, "on", dayStr, " ", timeStr)

		var day Date
		day.Set(dayStr)

		var gameStatus GameStatus
		switch {
		case strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT"):
			gameStatus = Finished
		case strings.Contains(timeStr, "AM") || strings.Contains(timeStr, "PM"):
			gameStatus = Future
		default:
			gameStatus = InProgress
		}

		game := Game{
			TeamV:  teamVStr,
			TeamH:  teamHStr,
			ScoreV: scoreVStr,
			ScoreH: scoreHStr,
			Day:    day,
			Time:   timeStr,
			Status: gameStatus,
		}

		switch gameStatus {
		case Future:
			allGamesFinal = false
		case InProgress:
			allGamesFinal = false
			gamesInProgress = true
		case Finished:
		}

		/* Convenient way to store dates of games without duplicates */
		gameTimes[day.Time().Unix()] = true

		season.Week[week].Games = append(season.Week[week].Games, game)
		gameIndex := len(season.Week[week].Games) - 1
		season.Week[week].teamToGame[teamVStr] = &season.Week[week].Games[gameIndex]
		season.Week[week].teamToGame[teamHStr] = &season.Week[week].Games[gameIndex]
	}

	/* Sort the start times by storing the keys (aka start times)
	 * in a slice in sorted order */
	var keys []int64
	for k := range gameTimes {
		keys = append(keys, k)
	}
	sort.Sort(ByInt64(keys))

	// Print the game times (w/o duplicates) in sorted order
	//    for _, k := range keys {
	//        fmt.Println("gameTimes ==>", time.Unix(k, 0))
	//    }
	season.Week[week].weekStart = time.Unix(keys[0], 0)
	season.Week[week].weekEnd = time.Unix(keys[len(keys)-1], 0)
	log.Println("Week Index", week, "Num", season.Week[week].Num,
		"start", season.Week[week].weekStart.Format("Mon Jan 2"),
		"end", season.Week[week].weekEnd.Format("Mon Jan 2"))
	log.Println("Games in progress = ", gamesInProgress)
	log.Println("All Games Final = ", allGamesFinal)
}

/**********************************************************/

func redirectToHttps(w http.ResponseWriter, r *http.Request) {
	// Redirect the incoming HTTP request.
	log.Println("redirectToHttps", r.RemoteAddr, r.RequestURI)
	http.Redirect(w, r, options.RedirectUrl+r.RequestURI, http.StatusFound)
}

/**********************************************************/

func main() {

	options = optionsFromFile()

	/* setup logging */
	/* TODO: append to log file */
	logFile, err := os.Create("fbScores.log")
	if err != nil {
		log.Println("cannot open fbScores.log")
		return
	}
	log.SetFlags(log.LstdFlags)
	log.SetOutput(logFile)

	log.Println("Program started")

	log.Println("Options", options)

	getUsers()

	fmt.Println("Season", season.Year)

	for week := 0; week < 17; week++ {
		var s string
		if options.ScheduleFromWeb {
			s = fmt.Sprintf("%s%d", options.ScheduleUrl, week+1)
		} else {
			s = fmt.Sprintf("%s%d.html", options.ScheduleUrl, week+1)
		}
		season.Week[week].Num = week + 1
		getSchedule(week, s)
		log.Println("Scheule for week", week, ":", season.Week[week])
		fmt.Println(season.Week[week])
		fmt.Println()
	}

	updateWeekIndex()

	updateUserScores()

	//	go updateGames()

	webSrv()

	log.Println("Program ended")
}
