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
    fmt.Printf("%q: %s\n", err, sqlStmt)
    //return
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

      fmt.Printf("<-%d, From:\t%s, Text: %s \n", msg.ID, msg.Chat, *msg.Text)

      r := regexp.MustCompile(`^/(?P<command>[a-zA-Z0-9]+)\s{0,1}(?P<text>.*)$`)
      result := r.FindStringSubmatch(*msg.Text)

      if len(result) == 3 {
        recipient := tbotapi.NewRecipientFromChat(msg.Chat)
        blacklisted := false
        command, opt := result[1], result[2]

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
            api.NewOutgoingMessage(recipient, text).Send()
          }
          return
        }

        // check blacklist before we do the final step
        for _, entry := range blacklist {
          if entry == command {
            blacklisted = true
          }
        }
        if blacklisted {
          api.NewOutgoingMessage(recipient, "The command is black-listed :(").Send()
          return
        }

        if strings.EqualFold(command, "add") && opt != "" {
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

          _, err := api.NewOutgoingMessage(recipient, text).Send()
          if err != nil {
            fmt.Printf("Error sending: %s\n", err)
            return
          }
        }

        err = rows.Err()
        if err != nil {
          fmt.Printf("%q\n", err)
        }
      }

      /*outMsg, err := api.NewOutgoingMessage(tbotapi.NewRecipientFromChat(msg.Chat), *msg.Text).Send()

      if err != nil {
        fmt.Printf("Error sending: %s\n", err)
        return
      }
      fmt.Printf("->%d, To:\t%s, Text: %s\n", outMsg.Message.ID, outMsg.Message.Chat, *outMsg.Message.Text)*/
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
