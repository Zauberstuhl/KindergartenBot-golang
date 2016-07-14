package main

import (
  "encoding/json"
  "database/sql"
  "github.com/mrd0ll4r/tbotapi"
  _ "github.com/mattn/go-sqlite3"
  "os"
  "fmt"
  "regexp"
  "strings"
  "strconv"
  "math/rand"
  "net/http"
  "./boilerplate"
)

type Config struct {
  Token string
}

func mapMultiVars(opt string, text string)(res string) {
  optArray := strings.Split(opt, " ")
  for i := 0; i < len(optArray); i++ {
    text = strings.Replace(text, "$"+strconv.Itoa(i+1), optArray[i], -1)
  }
  return text
}

func main() {
  blacklist := [10]string{
    "help",
    "add",
    "list",
    "stats",
    "random",
    "fixbot",
    "points",
    "wall",
    "quiz",
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

  // kindergarten table + index
  db.Exec(`CREATE
    TABLE kindergarten (
      text TEXT(255),
      chat TEXT(25),
      command TEXT(25),
      UNIQUE(chat, command)
      ON CONFLICT IGNORE
    );
  `)
  db.Exec(`CREATE
    INDEX index_kindergarten_chat
    ON kindergarten (chat);
  `)
  // kindergarten_points table + index
  db.Exec(`CREATE
    TABLE kindergarten_points (
      handle TEXT(255),
      points INT(11) DEFAULT 0,
      answer TEXT(255) DEFAULT NULL,
      last_played INT(11) DEFAULT (strftime('%s','now')),
      UNIQUE(handle)
    );
  `)
  db.Exec(`CREATE UNIQUE
    INDEX index_kindergarten_points_handle
    ON kindergarten_points (handle);
  `);

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
        helloSir, _ := regexp.MatchString("(?i)^(haii|hi|hey|hallo|hello|yo)", plainResult[0])
        if helloSir {
          api.NewOutgoingMessage(recipient, "Hello, Sir").Send()
          return
        }
        byeSir, _ := regexp.MatchString("(?i)^(bye|bb|cu|cya)", plainResult[0])
        if byeSir {
          api.NewOutgoingMessage(recipient, "A good day, Sir").Send()
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
            cmd_text = mapMultiVars(opt, cmd_text)
            text := fmt.Sprintf("/%s %s", cmd, cmd_text)
            api.NewOutgoingMessage(recipient, text).SetMarkdown(true).Send()
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
          if randNum == 0 {
            randNum = 1
          }
          text := fmt.Sprintf("You roll %d (1-%d)", randNum, rollTill)
          api.NewOutgoingMessage(recipient, text).Send()
          return
        }

        if strings.EqualFold(command, "wall") {
          rows, err := db.Query(`SELECT handle, points
            FROM kindergarten_points
            ORDER BY points DESC
            LIMIT 10;`)
          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()

          var wall string = ""
          var cnt int = 0
          for rows.Next() {
            var handle string
            var points int
            err = rows.Scan(&handle, &points)
            if err != nil {
              return
            }
            cnt = cnt + 1
            wall = fmt.Sprintf("%s\n%d. %s -> *%d*", wall, cnt, handle, points)
          }
          api.NewOutgoingMessage(recipient, wall).SetMarkdown(true).Send()
        }

        if strings.EqualFold(command, "points") {
          var handle = strconv.Itoa(msg.From.ID)
          if msg.From.Username != nil {
            handle = *(msg.From.Username)
          }

          selectStmt := fmt.Sprintf(`SELECT points
          FROM kindergarten_points
          WHERE handle like '%s';`, handle)
          rows, err := db.Query(selectStmt)

          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()

          points := 0
          for rows.Next() { rows.Scan(&points) }
          text := fmt.Sprintf("You have *%d* points!", points)

          api.NewOutgoingMessage(recipient, text).SetMarkdown(true).Send()
        }

        if strings.EqualFold(command, "quiz") {
          var handle = strconv.Itoa(msg.From.ID)
          if msg.From.Username != nil {
            handle = *(msg.From.Username)
          }

          selectStmt := fmt.Sprintf(`SELECT count(*) as count
          FROM kindergarten
          WHERE chat = %d;`, msg.Chat.ID)
          rows, err := db.Query(selectStmt)
          defer rows.Close()
          var count int = 0
          for rows.Next() { rows.Scan(&count) }
          if count < 50 {
            api.NewOutgoingMessage(recipient,
              "You need a minimum of 50 commands available for this command!").Send()
            return;
          }

          quizRegex := regexp.MustCompile(`(?i)^/quiz\s(?P<answer>.+)$`)
          quizResult := quizRegex.FindStringSubmatch(*msg.Text)
          if len(quizResult) == 2 {
            answer := execResult[2]

            selectStmt := fmt.Sprintf(`SELECT answer
              FROM kindergarten_points
              WHERE handle like '%s'
              AND answer like '%s';`, handle, answer)
            rows, err := db.Query(selectStmt)

            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
            defer rows.Close()

            correctAnswer := ""
            updateQuery := `UPDATE kindergarten_points
              SET points = points %s 1, answer = ''
              WHERE handle like '%s';`
            for rows.Next() { rows.Scan(&correctAnswer) }
            var result string = ""
            if correctAnswer == "" {
              result = "-"
            } else { result = "+" }

            api.NewOutgoingMessage(recipient,
              fmt.Sprintf("*%s*", result)).SetMarkdown(true).Send()
            updateQuery = fmt.Sprintf(updateQuery, result, handle)
            db.Exec(updateQuery)
            return
          }

          selectStmt = fmt.Sprintf(`SELECT points
          FROM kindergarten_points
          WHERE handle like '%s'`, handle)
          rows, err = db.Query(selectStmt)

          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()

          var exists int = -1
          var points int = -1
          for rows.Next() {
            exists = 1
            rows.Scan(&points)
          }

          if exists < 0 {
            insertStmt := fmt.Sprintf(`INSERT
            INTO kindergarten_points (handle)
            VALUES ('%s')`, handle)
            _, err := db.Exec(insertStmt)

            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
            points = 0
          }

          selectStmt = fmt.Sprintf(`SELECT text
            FROM kindergarten_points
            INNER JOIN kindergarten
            WHERE chat = %d
            AND handle like '%s'
            AND command like answer
            LIMIT 1;`, msg.Chat.ID, handle)
          rows, err = db.Query(selectStmt)
          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          defer rows.Close()
          var text string
          for rows.Next() { rows.Scan(&text) }

          if text == "" {
            // ELECT * FROM table ORDER BY RANDOM() LIMIT 1;
            selectStmt = fmt.Sprintf(`SELECT command, text
              FROM kindergarten
              WHERE chat LIKE '%d'
              ORDER BY RANDOM() LIMIT 1;`, msg.Chat.ID)
            fmt.Printf("query -> %s\n", selectStmt)

            rows, err = db.Query(selectStmt)
            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
            defer rows.Close()

            var command string
            for rows.Next() { rows.Scan(&command, &text) }
            if command == "" {
              fmt.Printf("Command nil aborting quiz!")
              return
            }

            insertStmt := fmt.Sprintf(`UPDATE kindergarten_points
            SET answer = '%s' WHERE handle like '%s';`, command, handle)
            _, err = db.Exec(insertStmt)

            if err != nil {
              fmt.Printf("%q\n", err)
              return
            }
          }

          api.NewOutgoingMessage(recipient, text).SetMarkdown(true).Send()
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
          text = mapMultiVars(opt, text)

          // Try uploading images
          imgRegex := regexp.MustCompile(`(?i)(?P<url>https?://.+/(?P<name>.+)\.(jpg|jpeg|png|gif))`)
          imgResult := imgRegex.FindStringSubmatch(text)
          if len(imgResult) == 4 {
            image_url, image_name, image_ext := imgResult[1], imgResult[2], imgResult[3]

            if strings.EqualFold(image_ext, "gif") {
              api.NewOutgoingMessage(recipient, text).Send()
              return
            }

            resp, err := http.Get(image_url)
            defer resp.Body.Close()

            if err != nil || resp.ContentLength > 1000000 {
              fmt.Printf("Content length exceeds 1000k: %d\n", resp.ContentLength)
              api.NewOutgoingMessage(recipient, text).Send()
              return
            }

            //fmt.Printf("%s: %s -> %s\n", image_url, image_name, image_ext)
            file_name := fmt.Sprintf("%s.%s", image_name, image_ext)
            api.NewOutgoingPhoto(recipient, file_name, resp.Body).Send()
          } else {
            api.NewOutgoingMessage(recipient, text).SetMarkdown(true).Send()
          }
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
