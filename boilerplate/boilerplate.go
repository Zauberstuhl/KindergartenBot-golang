package boilerplate

import (
  "github.com/mrd0ll4r/tbotapi"
  "fmt"
  "log"
  "os"
  "os/signal"
  "sync"
  "syscall"
)

// botFunc describes how the bot handles an update
type BotFunc func(update tbotapi.Update, api *tbotapi.TelegramBotAPI)

// runBot runs a bot
func RunBot(apiKey string, bot BotFunc, name, description string) {
  fmt.Printf("%s: %s\n", name, description)
  fmt.Println("Starting...")

  api, err := tbotapi.New(apiKey)
  if err != nil {
    log.Fatal(err)
  }

  // just to show its working
  fmt.Printf("User ID: %d\n", api.ID)
  fmt.Printf("Bot Name: %s\n", api.Name)
  fmt.Printf("Bot Username: %s\n", api.Username)

  closed := make(chan struct{})
  wg := &sync.WaitGroup{}

  wg.Add(1)
  go func() {
    defer wg.Done()
    for {
      select {
      case <-closed:
        return
      case update := <-api.Updates:
        if update.Error() != nil {
          fmt.Printf("Update error: %s\n", update.Error())
          continue
        }

        bot(update.Update(), api)
      }
    }
  }()

  // ensure a clean shutdown
  closing := make(chan struct{})
  shutdown := make(chan os.Signal)
  signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

  go func() {
    <-shutdown
    signal.Stop(shutdown)
    close(shutdown)
    close(closing)
  }()

  fmt.Println("Bot started. Press CTRL-C to close...")

  // wait for the signal
  <-closing
  fmt.Println("Closing...")

  // always close the API first, let it clean up the update loop
  api.Close() //this might take a while
  close(closed)
  wg.Wait()
}

