package main

import (
  "time"
  "errors"
  "encoding/json"
  "database/sql"
  "github.com/zauberstuhl/tbotapi"
  _ "github.com/mattn/go-sqlite3"
  "os"
  "fmt"
  "regexp"
  "strings"
  "strconv"
  "math/rand"
  "net/http"
  "github.com/Zauberstuhl/KindergartenBot-golang/boilerplate"
)

var openBan bool = false

var blacklist [13]string = [13]string{
  "help",
  "add",
  "list",
  "stats",
  "random",
  "fixbot",
  "ban",
  "banpool",
  "points",
  "wall",
  "quiz",
  "roll",
}

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

func ban(api *tbotapi.TelegramBotAPI, userID, channelID int, recipient tbotapi.Recipient, seconds int) error {
  timeNow := int(time.Now().Unix())

  ban := api.NewOutgoingRestrictChatMember(recipient, userID)
  ban.UntilDate = timeNow + seconds
  ban.CanSendMessages = false
  ban.CanSendMediaMessages = false
  ban.CanSendOtherMessages = false
  ban.CanAddWebPagePreviews = false

  db, err := sql.Open("sqlite3", "./kindergarten.db")
  if err != nil {
    fmt.Printf("%q\n", err)
    return errors.New("Wasn't able to open the database!")
  }
  defer db.Close()

  var last_updated int
  var query string
  err = db.QueryRow(`SELECT last_updated FROM kindergarten_ban_pool
    WHERE chat_id = ?
    AND user_id = ?
    LIMIT 1`, channelID, userID).Scan(&last_updated)
  if err == nil {
    // challenge accepted
    if last_updated > (timeNow - 320) {
      query = `UPDATE kindergarten_ban_pool
        SET seconds = 0 WHERE chat_id = %d AND user_id = %d;`
      db.Exec(fmt.Sprintf(query, channelID, userID))
      return errors.New("Cheating detected.. Reseting points!")
    } else {
      query = `UPDATE kindergarten_ban_pool
        SET last_updated = %d, seconds = seconds + %d WHERE chat_id = %d AND user_id = %d;`
    }
  } else {
    query = `INSERT INTO kindergarten_ban_pool
      (last_updated, seconds, chat_id, user_id) VALUES (%d, %d, %d, %d);`
  }
  db.Exec(fmt.Sprintf(query, timeNow, seconds, channelID, userID))
  return ban.Send()
}

func updateBot(update tbotapi.Update, api *tbotapi.TelegramBotAPI) {
  db, err := sql.Open("sqlite3", "./kindergarten.db")
  if err != nil {
    fmt.Printf("%q\n", err)
    return
  }
  defer db.Close()

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

    // get user information
    var f, l, u string
    if msg.Chat.FirstName != nil {
      f = *msg.Chat.FirstName
    }
    if msg.Chat.LastName != nil {
      l = *msg.Chat.LastName
    }
    if msg.Chat.Username != nil {
      u = *msg.Chat.Username
    }
    // check if user is known
    var lastName, firstName, username, query string
    err = db.QueryRow(`SELECT last_name, first_name, username
      FROM kindergarten_users
      WHERE chat_id = ?
      AND user_id = ?
      LIMIT 1`, msg.Chat.ID, msg.From.ID).Scan(&lastName, &firstName, &username)
    if err == nil {
      if f != firstName  || l != lastName || u != username {
        query = `UPDATE kindergarten_users
          SET last_name = "%s", first_name = "%s", username = "%s"
          WHERE chat_id = %d AND user_id = %d;`
      }
    } else {
      query = `INSERT INTO kindergarten_users
        (last_name, first_name, username, chat_id, user_id) VALUES ("%s", "%s", "%s", %d, %d);`
    }
    db.Exec(fmt.Sprintf(query, l, f, u, msg.Chat.ID, msg.From.ID))

    randSource := rand.NewSource(time.Now().UnixNano())
    randWithSource := rand.New(randSource)
    if randWithSource.Intn(20) == 0 {
      banTime := 320 + randWithSource.Intn(3380)
      err := ban(api, msg.From.ID, msg.Chat.ID, recipient, banTime)
      if err == nil {
        text := `You are one out of 20.. Welcome to the ban-list for %d seconds!`
        api.NewOutgoingMessage(recipient, fmt.Sprintf(text, banTime)).Send()
        return
      } else {
        fmt.Printf("%q\n", err)
      }
    }

    plainRegex := regexp.MustCompile(`^[^/](?P<text>.+?)$`)
    plainResult := plainRegex.FindStringSubmatch(*msg.Text)
    if len(plainResult) == 2 {
      words := strings.Split(plainResult[0], " ")
      for _, word := range words {
        var text string
        err = db.QueryRow(`SELECT text FROM kindergarten
          WHERE command LIKE ?
          AND chat LIKE ?
          AND match = 1
          LIMIT 1`, word, msg.Chat.ID).Scan(&text)

        if err == nil {
          api.NewOutgoingMessage(recipient, text).Send()
          break
        }
      }
      return
    }

    addRegex := regexp.MustCompile(`^/(?P<add>add|match)\s(?P<command>[a-zA-Z0-9]+)\s(?P<text>.+)$`)
    addResult := addRegex.FindStringSubmatch(*msg.Text)
    if len(addResult) == 4 {
      add, command, opt := addResult[1], addResult[2], addResult[3]

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

      // choose between text match or command
      match := 1
      if add == "add" {
        match = 0
      }

      insertStmt := fmt.Sprintf(`
        INSERT INTO kindergarten (chat, command, text, match)
        VALUES ('%d', '%s', '%s', %d)
      `, msg.Chat.ID, command, opt, match)
      _, err = db.Exec(insertStmt)
      if err != nil {
        fmt.Printf("%q\n", err)
        return
      }
      api.NewOutgoingMessage(recipient, "New command '"+command+"' was added!").Send()
      return
    }

    execRegex := regexp.MustCompile(`^/(?P<command>[a-zA-Z0-9\_]+)\s{0,1}(?P<text>.*)$`)
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
          AND match = 0
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

      if strings.EqualFold(command, "banpool") {
        rows, err := db.Query(`SELECT kindergarten_ban_pool.user_id,
            username, last_name, first_name,
            seconds, used, (seconds - used) AS current
          FROM kindergarten_ban_pool
          LEFT JOIN kindergarten_users
            ON kindergarten_users.user_id = kindergarten_ban_pool.user_id
          WHERE kindergarten_ban_pool.chat_id = ?
          GROUP BY kindergarten_ban_pool.user_id
          ORDER BY current DESC
          LIMIT 30;`, msg.Chat.ID)
        if err != nil {
          fmt.Printf("%q\n", err)
          return
        }
        defer rows.Close()

        var pool string
        var cnt int
        for rows.Next() {
          var username, first_name, last_name sql.NullString
          var userID, seconds, used, current int
          err = rows.Scan(&userID,
            &username, &last_name, &first_name,
            &seconds, &used, &current)
          if err != nil {
            fmt.Printf("%q\n", err)
            return
          }
          cnt = cnt + 1
          name := first_name.String+" "+last_name.String+" ("+username.String+")"
          if name == "  ()" {
            pool = fmt.Sprintf("%s\n%d. %d -> *%d* (all: %d, used: %d)",
              pool, cnt, userID, current, seconds, used)
          } else {
            pool = fmt.Sprintf("%s\n%d. %s -> *%d* (all: %d, used: %d)",
              pool, cnt, name, current, seconds, used)
          }
        }
        api.NewOutgoingMessage(recipient, pool).SetMarkdown(true).Send()
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
        WHERE chat = %d AND match = 0;`, msg.Chat.ID)
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
          // SELECT * FROM table ORDER BY RANDOM() LIMIT 1;
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

      if openBan || strings.EqualFold(command, "ban") {
        userID := msg.From.ID
        if !openBan && randWithSource.Intn(2) == 0 {
          openBan = true
          return
        } else {
          fmt.Printf("<-%d, From:\t%s, was banned! \n", msg.ID, msg.Chat)
          openBan = false
        }

        // ban the user for min 60 seconds or max four hours
        banTime := 320 + randWithSource.Intn(14080)
        err := ban(api, userID, msg.Chat.ID, recipient, banTime)
        if err == nil {
          text := `You are banned for %d seconds! ¯\_(ツ)_/¯`
          api.NewOutgoingMessage(recipient, fmt.Sprintf(text, banTime)).Send()
          return
        } else {
          fmt.Printf("%q\n", err)
        }
      }

      // still not finished?
      // then try fetching the right text
      // for the command from the DB

      selectStmt := fmt.Sprintf(`
        SELECT text
        FROM kindergarten
        WHERE command LIKE '%s'
        AND chat LIKE '%d'
        AND match = 0
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
          if err != nil || resp.ContentLength > 1000000 {
            //fmt.Printf("Content length exceeds 1000k: %d\n", resp.ContentLength)
            api.NewOutgoingMessage(recipient, "Your image size is over 9000!!").Send()
            return
          }
          defer resp.Body.Close()

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

func main() {
  file, _ := os.Open("./config.json")
  decoder := json.NewDecoder(file)
  config := Config{}
  err := decoder.Decode(&config)

  if err != nil {
    fmt.Printf("Error reading configuration file: %q\n", err)
    return
  }
  fmt.Printf("Telegram API Key: %s\n", config.Token)

  // run the bot, this will block
  boilerplate.RunBot(config.Token, updateBot, "Kindergarten", "Still in Kindergarten?")
}
