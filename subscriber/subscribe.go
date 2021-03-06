package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/nictuku/stardew-rocks/parser"
	"github.com/nictuku/stardew-rocks/view"

	"github.com/streadway/amqp"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func main() {
	conn, err := amqp.Dial("amqp://guest:guest@amqp.stardew.rocks:5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	err = ch.ExchangeDeclare(
		"OtherFiles-1", // name
		"fanout",       // type
		false,          // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	)
	failOnError(err, "Failed to declare an exchange")

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when usused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	err = ch.QueueBind(
		q.Name,         // queue name
		"",             // routing key
		"OtherFiles-1", // exchange
		false,
		nil)
	failOnError(err, "Failed to bind a queue")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	count := 0

	farmMap := parser.LoadFarmMap()

	go func() {
		for d := range msgs {
			count++

			saveGame, err := parser.ParseSaveGame(bytes.NewReader(d.Body))
			if err != nil {
				log.Print(err)
				return
			}
			if saveGame.Player.Name == "" {
				log.Printf("blank player name")
				break
			}
			_, name := path.Split(path.Clean(saveGame.Player.Name)) // please don't hacko me mister

			ts := time.Now().Unix()

			// Write the save game, then write the screenshot.
			// TODO: deal with races and conflicts.
			saveFile := path.Join(wwwDir(), "saveGames", fmt.Sprintf("%v-%d.xml", name, ts))
			sf, err := os.OpenFile(saveFile, os.O_CREATE|os.O_WRONLY, 0666)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := sf.Write(d.Body); err != nil {
				log.Printf("failed to write save file at %v: %v", saveFile, err)
			} else {
				log.Printf("Wrote saveGame file %v", saveFile)
			}
			sf.Close()

			mapFile := path.Join(wwwDir(), fmt.Sprintf("map-%v-%d.png", name, ts))
			f, err := os.OpenFile(mapFile, os.O_CREATE|os.O_WRONLY, 0666)
			if err != nil {
				log.Fatal(err)
			}
			lastSaveMu.Lock()
			lastSave = d.Body
			lastSaveMu.Unlock()
			view.WriteImage(farmMap, saveGame, f)
			f.Close()
			log.Printf("Wrote map file %v", mapFile)
			log.Printf("Total messages so far: %d", count)
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	RunHTTPServer()
}
