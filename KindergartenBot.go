package main

import (
  "encoding/json"
  "database/sql"
  "bitbucket.org/mrd0ll4r/tbotapi"
  _ "github.com/mattn/go-sqlite3"
  "os"
  "fmt"
  "regexp"
  "strings"
  "strconv"
  "math/rand"
  "./boilerplate"
)

type Config struct {
  Token string
}

func main() {
  blacklist := [7]string{
    "help",
    "add",
    "list",
    "stats",
    "random",
    "fixbot",
    "roll",
  }

  file, _ := os.Open("./config.json")
  decoder := json.NewDecoder(file)
  config := Config{}
  err := decoder.Decode(&config)

  if err != nil {
    fmt.Printf("Error reading configuration file: %q\n", err)
    return
  }
  fmt.Printf("Telegram API Key: %s\n", config.Token)

  db, err := sql.Open("sqlite3", "./kindergarten.db")
  if err != nil {
    fmt.Printf("%q\n", err)
    return
  }
  defer db.Close()

  sqlStmt := `
    CREATE TABLE kindergarten (
      text TEXT(255),
      chat TEXT(25),
      command TEXT(25),
      UNIQUE(chat, command)
      ON CONFLICT IGNORE
    );`

  _, err = db.Exec(sqlStmt)
  if err != nil {
    // print but do not return since the
    // table could exist already!
    fmt.Printf("%q: %s\n", err, sqlStmt)
  }

  updateFunc := func(update tbotapi.Update, api *tbotapi.TelegramBotAPI) {
    switch update.Type() {
    case tbotapi.MessageUpdate:
      msg := update.Message
      typ := msg.Type()
      if typ != tbotapi.TextMessage {
        fmt.Println("Ignoring non-text message")
        return
      }
      recipient := tbotapi.NewRecipientFromChat(msg.Chat)

      fmt.Printf("<-%d, From:\t%s, Text: %s \n", msg.ID, msg.Chat, *msg.Text)

      plainRegex := regexp.MustCompile(`^[^/](?P<text>.+?)$`)
      plainResult := plainRegex.FindStringSubmatch(*msg.Text)
      if len(plainResult) == 2 {
        helloSir, _ := regexp.MatchString("^(hi|hey|hallo|hello|yo)$", plainResult[0])
        if helloSir {
          api.NewOutgoingMessage(recipient, "Hello, Sir").Send()
          return
        }
        // to be continue
        return
      }

      addRegex := regexp.MustCompile(`^/add\s(?P<command>[a-zA-Z0-9]+)\s(?P<text>.+)$`)
      addResult := addRegex.FindStringSubmatch(*msg.Text)
      if len(addResult) == 3 {
        command, opt := addResult[1], addResult[2]

        blacklisted := false
        for _, entry := range blacklist {
          if entry == command {
            blacklisted = true
          }
        }
        if blacklisted {
          api.NewOutgoingMessage(recipient, "The "+command+" is black-listed :(").Send()
          return
        }

        insertStmt := fmt.Sprintf(`
          INSERT INTO kindergarten (chat, command, text)
          VALUES ('%d', '%s', '%s')
        `, msg.Chat.ID, command, opt)
        _, err = db.Exec(insertStmt)
        if err != nil {
          fmt.Printf("%q\n", err)
          return
        }
        api.NewOutgoingMessage(recipient, "New command '"+command+"' was added!").Send()
        return
      }

      execRegex := regexp.MustCompile(`^/(?P<command>[a-zA-Z0-9]+)\s{0,1}(?P<text>.*)$`)
      execResult := execRegex.FindStringSubmatch(*msg.Text)
      if len(execResult) == 3 {
        command, opt := execResult[1], execResult[2]

        if strings.EqualFold(command, "random") || strings.EqualFold(command, "rnd") {
          selectStmt := fmt.Sprintf(`
            SELECT command, text
            FROM kindergarten
            WHERE chat
            LIKE '%d'
            AND _ROWID_ >= (abs(random()) %% (SELECT max(_ROWID_) FROM kindergarten))
            LIMIT 1
          `, msg.Chat.ID)

          rows, err := db.Query(selectStmt)
          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()
          for rows.Next() {
            var cmd string
            var cmd_text string
            err = rows.Scan(&cmd, &cmd_text)
            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
            text := fmt.Sprintf("/%s %s", cmd, cmd_text)
            text = strings.Replace(text, "$1", opt, -1)
            api.NewOutgoingMessage(recipient, text).Send()
          }
          return
        }

        if strings.EqualFold(command, "stats") {
          selectStmt := fmt.Sprintf(`
            SELECT count(*) as 'count'
            FROM kindergarten
            WHERE chat LIKE '%d'
          `, msg.Chat.ID)

          rows, err := db.Query(selectStmt)
          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()
          for rows.Next() {
            var count int
            err = rows.Scan(&count)
            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
            text := fmt.Sprintf(`There are %d commands available!`, count)
            api.NewOutgoingMessage(recipient, text).Send()
          }
          return
        }

        if strings.EqualFold(command, "roll") {
          rollTill := 10
          if opt != "" {
            i, err := strconv.Atoi(opt)
            if err != nil {
              return
            }
            rollTill = i
          }
          randNum := rand.Intn(rollTill)
          text := fmt.Sprintf("You roll %d(0-%d)", randNum, rollTill)
          api.NewOutgoingMessage(recipient, text).Send()
          return
        }

        // still not finished?
        // then try fetching the right text
        // for the command from the DB

        selectStmt := fmt.Sprintf(`
          SELECT text
          FROM kindergarten
          WHERE command LIKE '%s'
          AND chat LIKE '%d'
          LIMIT 1
        `, command, msg.Chat.ID)

        rows, err := db.Query(selectStmt)
        if err != nil {
          fmt.Printf("%q\n", err)
          return
        }
        defer rows.Close()

        for rows.Next() {
          var text string
          err = rows.Scan(&text)
          if err != nil {
            fmt.Printf("%q\n", err)
          }
          text = strings.Replace(text, "$1", opt, -1)
          api.NewOutgoingMessage(recipient, text).Send()
        }

        err = rows.Err()
        if err != nil {
          fmt.Printf("%q\n", err)
        }
        return
      }
    case tbotapi.InlineQueryUpdate:
      fmt.Println("Ignoring received inline query: ", update.InlineQuery.Query)
    case tbotapi.ChosenInlineResultUpdate:
      fmt.Println("Ignoring chosen inline query result (ID): ", update.ChosenInlineResult.ID)
    default:
      fmt.Printf("Ignoring unknown Update type.")
    }
  }

  // run the bot, this will block
  boilerplate.RunBot(config.Token, updateFunc, "Kindergarten", "Still in Kindergarten?")
}