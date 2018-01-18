package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/proxa/GoBot/markov"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

// name of the bot, this should probably go into a configuration file at some point.
var botName = "UncleJim"
var db *sql.DB
var serv = flag.String("server", "chat.freenode.net:6667", "hostname and port for irc server to connect to")
var nick = flag.String("nick", botName, "nickname for the bot")

// this regex matches highlights, and avoids adding them to the database.
var highlightRegex = regexp.MustCompile(`^[^\s]+:.*$`)

func main() {
	flag.Parse()

	// connect to sql database named 'gobot'
	db, err := sql.Open("mysql", "gobot:test@/gobot?charset=utf8")
	checkErr(err)

	// create database table and instantiate database object ONE TIME.
	createTable(db)

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = true
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = []string{"#afterlunch"}
	}
	irc, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	if err != nil {
		panic(err)
	}

	irc.AddTrigger(LogMessage)
	irc.AddTrigger(MarkovChain)
	irc.Logger.SetHandler(log.StdoutHandler)

	irc.Run()
	fmt.Println("Bot shutting down.")
}

// LogMessage logs all messages from chat to the database for chaining later.
var LogMessage = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		/* This ignores server messages, the bot's messages, messages from null senders (happens apparently),
		   messages that are commands to this bot, messages that are commands for my other bot, all messages
		   from my other bot, quit messages, and more commands for my other bot.  Whew. */
		return !strings.Contains(m.From, ".") && m.From != botName && m.From != "" &&
			!strings.HasPrefix(m.Content, "-") && !strings.HasPrefix(m.Content, "!") &&
			m.From != "buttbutt" && !strings.HasPrefix(m.Content, "Quit:") &&
			!strings.HasPrefix(m.Content, "~")
	},
	func(irc *hbot.Bot, m *hbot.Message) bool {
		writeMessageToDatabase(db, m.Content)
		checkRandomResponseTime(irc, m)
		return true
	},
}

// MarkovChain is the on-demand way to start the markov chain.
var MarkovChain = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		return m.Command == "PRIVMSG" && m.Content == "-mkv"
	},
	func(irc *hbot.Bot, m *hbot.Message) bool {
		reply := getMarkovText(db)
		irc.Reply(m, reply)
		return false
	},
}

func checkRandomResponseTime(irc *hbot.Bot, m *hbot.Message) {
	number := rand.Intn(100)
	if number <= 5 {
		// spin off a new thread to randomly chat sometime in the next 3 hours
		go func() {
			sleeptime := rand.Intn(180)
			time.Sleep(time.Duration(sleeptime) * time.Minute)
			reply := getMarkovText(db)
			irc.Reply(m, reply)
		}()
	} else if number < 6 {
		reply := getMarkovText(db)
		irc.Reply(m, reply)
	}
}

func getMarkovText(db *sql.DB) string {
	data := getMessageFromDatabase(db) // problem: could be empty string
	// randomize the length
	length := rand.Intn(50)
	length++
	result := markov.DoMarkovChain(data, length)
	return result
}

// this opens the connection to the database
func createTable(db *sql.DB) {
	// create the table creation string
	createTable := string("CREATE TABLE IF NOT EXISTS `messages` (`message` VARCHAR(450) NOT NULL);")

	// prepare, check for error, and defer close
	stmt, err := db.Prepare(createTable)
	checkErr(err)
	res, err := stmt.Exec()
	checkErr(err)
	fmt.Println(res)
}

func writeMessageToDatabase(db *sql.DB, msg string) {
	// trim any beginning or trailing whitespace
	trimmed := strings.TrimSpace(msg)

	// replace all action text with /me
	replaced := strings.Replace(trimmed, "ACTION", "/me", -1)

	split := strings.Fields(replaced)
	// if message is only one word (or none), don't bother adding it because it can't be chained
	if len(split) <= 1 {
		return
	}

	// if message contains highlight, remove it
	if highlightRegex.MatchString(split[0]) {
		log.Debug("Found highlight message: ", string(replaced))
		return
	}

	// add to database
	err := db.Ping()
	if err == nil {
		stmt, err := db.Prepare("INSERT messages SET message=?")
		defer stmt.Close()
		if err == nil {
			res, err := stmt.Exec(replaced)
			if err == nil {
				fmt.Println("Result from database: ", res)
			}
		} else {
			fmt.Println("Error preparing SQL statement: ", err)
		}
	} else {
		fmt.Println("Error connecting to mysql database: ", err)
	}
}

func getMessageFromDatabase(db *sql.DB) string {
	err := db.Ping()
	checkErr(err)
	rows, err := db.Query("SELECT * FROM messages ORDER BY RAND()")
	if err != nil {
		fmt.Println(err)
		return ""
	}
	var tmp string
	defer rows.Close()
	var buffer bytes.Buffer
	for rows.Next() {
		err := rows.Scan(&tmp)
		buffer.WriteString(tmp + " ")
		if err != nil {
			fmt.Println(err)
		}
	}
	err = rows.Err()
	if err != nil {
		fmt.Println(err)
	}
	return buffer.String()
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}