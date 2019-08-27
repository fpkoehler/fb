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
	ScheduleUrl     string
	UpdateUrl       string
	PwRecoverSecret string
	HostWhiteList   string
	AdminEmail      string
	AdminEmailPw    string
}

type GameStatus int

const (
	Future GameStatus = iota
	InProgress
	Finished
)

const numberOfWeeks = 5

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
	Week [numberOfWeeks]Week
}

type Selection struct {
	Team       string
	Confidence int
	When       string // time the selection was made
}

type UserWeek struct {
	Num        int `xml:"Week,attr"`
	Points     int
	GoodPicks  int
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
	GoodPicks   int
}

/* For sorting the standings, note we want
 * ascending order, so "Less" actually returns greater */
type ByStandingRow []StandingRow

func (a ByStandingRow) Len() int           { return len(a) }
func (a ByStandingRow) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByStandingRow) Less(i, j int) bool { return a[i].Total > a[j].Total }

/**********************************************************/

var options Options

var season = Season{Year: 2019}

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
		"LA Chargers":          "Chargers",
		"Los Angeles Chargers": "Chargers",
		"LA Rams":              "Rams",
		"Los Angeles Rams":     "Rams",
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
		goodPicks := 0
		weeksPlayed := 0
		totalForUser := 0
		aveScoreStr := "0.0"

		for i := 0; i < lastIWeek; i++ {
			goodPicks += u.UserWeeks[i].GoodPicks
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

		x := StandingRow{
			Name:        u.Name,
			Total:       totalForUser,
			WeeksPlayed: weeksPlayed,
			WeeksWon:    weeksWon,
			AvePerWeek:  aveScoreStr,
			GoodPicks:   goodPicks,
		}
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

		gamesInProgress := false
		gameTimes := make(map[int64]bool)

		iter := gameSchPageIterator{}

		var err error
		var file *os.File
		var resp *http.Response
		if options.UpdateFromWeb {
			url := fmt.Sprintf("%s%d", options.ScheduleUrl, iWeek+1)
			log.Println("Updating games for week indx", iWeek, "from", url, ":")
			resp, err = http.Get(url)
			for err != nil {
				log.Println("Error from http.Get, retry in 1 minute:", err.Error())
				time.Sleep(1 * time.Minute)
				resp, err = http.Get(url)
			}
			iter.p.Init(resp.Body)
		} else {
			fileName := fmt.Sprintf("%s%d.html", options.ScheduleUrl, iWeek+1)
			log.Println("Updating games for week indx", iWeek, "from", fileName, ":")
			file, err = os.Open(fileName) // For read access.
			if err != nil {
				log.Println("cannot open ", fileName)
				return
			}
			iter.p.Init(file)
		}

		for iter.Next() {
			game := iter.game

			switch game.Status {
			case Future:
				t := game.Day.AddDayTime(game.Time)
				fmt.Println("game starts", t.Format(time.UnixDate), "from now", t.Sub(time.Now()))
				log.Println("game starts", t.Format(time.UnixDate), "from now", t.Sub(time.Now()))
				/* store start time in a map. This is a convenient way to
				 * keep track of start times for the week without duplicates */
				gameTimes[t.Unix()] = true
			case InProgress:
				gamesInProgress = true
			case Finished:
			}

			pGame := season.Week[iWeek].teamToGame[game.TeamV]
			if pGame == nil {
				_, _, line, _ := runtime.Caller(0)
				fmt.Println("line", line, "Could not find game for team", game.TeamV)
				log.Println("line", line, "Could not find game for team", game.TeamV)
				continue
			}

			// Update the game in season.Week[iWeek].Games[]
			*pGame = game
		}

		/* Done with parsing the page, a little cleanup */
		if options.UpdateFromWeb {
			resp.Body.Close()
		} else {
			file.Close()
		}

		updateUserScoresWeekIndex(iWeek)

		/* Compute the next time to loop */

		/* The schedule page often does not update the games
		 * while they are in progress.  So while the games is
		 * being played, the start time for the game is still
		 * listed.  When the game is over, the "time" for the
		 * game is changed to FINAL.
		 *
		 * So the strategy is to check back 3 hours after the
		 * start of the game.  Then if a start time is earlier
		 * then current time (the game(s) are in progress) check
		 * back in 15 minutes.  If a start time is later, that
		 * means there is another game starting later in the day,
		 * so check back in 3 hours. */

		/* Default is 8am the next day */
		nextTime := time.Now().AddDate(0, 0, 1) // add one day
		y, m, d := nextTime.Date()
		loc, _ := time.LoadLocation("America/Los_Angeles")
		nextTime = time.Date(y, m, d, 8, 0, 0, 0, loc)

		/* Get the earliest start time listed (if any) */
		nowUnix := time.Now().Unix()
		nextGameTodayStart := time.Time{} // zero
		for k := range gameTimes {
			if time.Unix(k, 0).YearDay() == time.Unix(nowUnix, 0).YearDay() {
				/* game time is today */
				if nextGameTodayStart.IsZero() {
					nextGameTodayStart = time.Unix(k, 0)
					continue
				}
				if time.Unix(k, 0).Before(nextGameTodayStart) {
					nextGameTodayStart = time.Unix(k, 0)
				}
			}
		}

		if !nextGameTodayStart.IsZero() {
			if nextGameTodayStart.Before(time.Now()) {
				/* The game started earlier, so check back in 15 minutes */
				log.Println("game started earlier:", nextGameTodayStart, "check back soon")
				nextTime = time.Now().Add(15 * time.Minute)
			} else {
				/* Check back 3 hours after the start time */
				log.Println("another game today starts", nextGameTodayStart)
				nextTime = nextGameTodayStart.Add(3 * time.Hour)
			}
		}

		/* In the rare cases where the schedule page shows in progress updates */
		/* If games in progress, then in a few minutes */
		if gamesInProgress {
			nextTime = time.Now().Add(30 * time.Minute)
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
		goodPicks := 0
		totalPoints := 0
		for _, s := range u.UserWeeks[iw].Selections {
			game := season.Week[iw].teamToGame[s.Team]
			if game == nil {
				_, _, line, _ := runtime.Caller(0)
				fmt.Println("line", line, "iWeek", iw, "Could not find game for user", u.Email, "selection", s.Team)
				log.Println("line", line, "iWeek", iw, "Could not find game for user", u.Email, "selection", s.Team)
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
				if points > 0 {
					goodPicks++
				}

				log.Printf("\t%s %s %s user %s %d points\n", team1, verb, team2, u.Email, points)
			}
		}

		u.UserWeeks[iw].GoodPicks = goodPicks
		if u.UserWeeks[iw].Points == totalPoints {
			log.Printf("user %s weekIndx %d totalPoints %d unchanged\n", u.Email, iw, totalPoints)
		} else {
			u.UserWeeks[iw].Points = totalPoints
			log.Printf("user %s weekIndx %d totalPoints %d goodPicks %d\n", u.Email, iw, totalPoints, goodPicks)
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

type gameSchPageIterator struct {
	p      ParseHTML
	game   Game
	dayStr string
}

func (iter *gameSchPageIterator) Next() bool {
	var timeStr string
	var teamHStr string
	var teamVStr string
	var scoreVStr string
	var scoreHStr string
	var b bool

	if iter.p.SeekTag("divider", "left") == false {
		return false
	}

	if strings.Compare("divider", iter.p.Tok.Attr[0].Val) == 0 {
		// section for games on this day
		iter.dayStr = iter.p.GetText()

		// advances to game date which will be same as iter.dayStr
		iter.p.SeekTag("left")
	}

	iter.p.SeekTag("center", "right")

	if strings.Compare("right", iter.p.Tok.Attr[0].Val) == 0 {
		// Game time
		timeStr = iter.p.GetText()
	} else {
		// center tag.
		// <th class="center" style="width:10%;">FINAL</th></tr>
		timeStr = iter.p.GetText()
	}

	scoreVStr = ""
	scoreHStr = ""

	iter.p.SeekTag("left")
	teamVStr = toTeam[iter.p.GetText()]

	if strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT") {
		scoreVStr, b = iter.p.SeekBoldText()
		if !b {
			fmt.Println("SeekBoldText() failed after teamVStr", teamVStr)
			return false
		}
	}

	iter.p.SeekTag("left")
	teamHStr = toTeam[iter.p.GetText()]

	if strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT") {
		scoreHStr, b = iter.p.SeekBoldText()
		if !b {
			fmt.Println("SeekBoldText() failed after teamHStr", teamHStr)
			return false
		}
	}

	log.Println(teamVStr, scoreVStr, "vs", teamHStr, scoreHStr, "on", iter.dayStr, " ", timeStr)

	var day Date
	day.Set(iter.dayStr)

	var gameStatus GameStatus
	switch {
	case strings.Contains(timeStr, "FINAL") || strings.Contains(timeStr, "F/OT"):
		gameStatus = Finished
	case strings.Contains(timeStr, "AM") || strings.Contains(timeStr, "PM"):
		gameStatus = Future
	default:
		gameStatus = InProgress
	}

	iter.game = Game{
		TeamV:  teamVStr,
		TeamH:  teamHStr,
		ScoreV: scoreVStr,
		ScoreH: scoreHStr,
		Day:    day,
		Time:   timeStr,
		Status: gameStatus,
	}

	return true
}

/*****************************************************************************/

func getSchedule(week int, url string) {

	allGamesFinal := true
	gamesInProgress := false
	gameTimes := make(map[int64]bool)

	season.Week[week].Games = make([]Game, 0, 16)
	season.Week[week].teamToGame = make(map[string]*Game)

	log.Println("Getting games for week indx", week, "from", url, ":")

	iter := gameSchPageIterator{}

	if options.ScheduleFromWeb {
		resp, err := http.Get(url)
		if err != nil {
			log.Println("Error from http.Get:", err.Error())
			return
		}
		defer resp.Body.Close()
		iter.p.Init(resp.Body)
	} else {
		fileName := url
		file, err := os.Open(fileName) // For read access.
		if err != nil {
			log.Println("cannot open ", fileName)
			return
		}
		defer file.Close()
		iter.p.Init(file)
	}

	for iter.Next() {
		game := iter.game
		switch game.Status {
		case Future:
			allGamesFinal = false
		case InProgress:
			allGamesFinal = false
			gamesInProgress = true
		case Finished:
		}

		/* Convenient way to store dates of games without duplicates */
		gameTimes[game.Day.Time().Unix()] = true

		season.Week[week].Games = append(season.Week[week].Games, game)
		gameIndex := len(season.Week[week].Games) - 1
		season.Week[week].teamToGame[game.TeamV] = &season.Week[week].Games[gameIndex]
		season.Week[week].teamToGame[game.TeamH] = &season.Week[week].Games[gameIndex]
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

	for week := 0; week < numberOfWeeks; week++ {
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

	go updateGames()

	webSrv()

	log.Println("Program ended")
}
