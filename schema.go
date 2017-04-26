package main

import (
  "fmt"
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
)

func init() {
  db, err := sql.Open("sqlite3", "./kindergarten.db")
  if err != nil {
    fmt.Printf("%q\n", err)
    return
  }
  defer db.Close()

  // kindergarten table + index
  db.Exec(`ALTER TABLE kindergarten RENAME TO kindergarten_tmp;`)
  db.Exec(`CREATE
    TABLE kindergarten (
      text TEXT(255),
      chat TEXT(25),
      command TEXT(25),
      match INTEGER(1) DEFAULT 0,
      UNIQUE(chat, command, match)
      ON CONFLICT IGNORE
    );
  `)
  db.Exec(`CREATE
    INDEX index_kindergarten_chat
    ON kindergarten (chat);
  `)
  if _, err = db.Exec(`INSERT INTO kindergarten (
      text, chat, command
    ) SELECT * FROM kindergarten_tmp;
  `); err == nil {
    db.Exec(`DROP TABLE kindergarten_tmp;`)
  }
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
}