// go: generate goversioninfo -icon = clock-icon.png
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

	// "fyne.io/fyne"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

var (
	Version = "dev"
)

func createDatabaseAndTable() {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sqlStmt := `
    CREATE TABLE IF NOT EXISTS stempelzeiten (
		datum STRING PRIMARY KEY,
        einstempelzeit STRING,
        ausstempelzeit STRING,
        arbeitszeit REAL,
        eingetragen INTEGER
    );
    `
	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}

}

func addDateIfNotExists() {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	date := time.Now().Format("02.01.2006")

	// Check if date already exists
	row := db.QueryRow("SELECT datum FROM stempelzeiten WHERE datum = ?", date)
	var existingDate string
	err = row.Scan(&existingDate)

	if err != nil && err != sql.ErrNoRows {
		fmt.Println("Error checking for existing date:", err)
		return
	}

	// If date does not exist, insert it
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO stempelzeiten (datum, eingetragen, arbeitszeit) VALUES (?, 0, 0)", date)
		if err != nil {
			fmt.Println("Error inserting date:", err)
			return
		}
		fmt.Println("Date added:", date)
	} else {
		fmt.Println("Date already exists:", existingDate)
	}
}

func writeCurrentTime(x string) {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	currentTime := time.Now().Format("15:04")
	date := time.Now().Format("02.01.2006")

	// Check if the field is NULL
	row := db.QueryRow(fmt.Sprintf("SELECT %s FROM stempelzeiten WHERE datum = ?", x), date)
	var existingValue sql.NullString
	err = row.Scan(&existingValue)
	if err != nil && err != sql.ErrNoRows {
		fmt.Println("Error checking for existing value:", err)
		return
	}
	if existingValue.Valid {
		fmt.Println("Value already exists:", existingValue.String)
		return
	}
	// Prepare the SQL statement
	stmt, err := db.Prepare(fmt.Sprintf("UPDATE stempelzeiten SET %s = ? WHERE datum = ?", x))

	if err != nil {
		fmt.Println("Error preparing statement:", err)
		return
	}
	defer stmt.Close()

	// Execute the SQL statement
	_, err = stmt.Exec(currentTime, date)
	if err != nil {
		fmt.Println("Error executing statement:", err)
		return
	}

	fmt.Println("Current time added to", x)
	db.Close()
}

func calculateArbeitszeit() {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT datum, einstempelzeit, ausstempelzeit FROM stempelzeiten WHERE arbeitszeit IS NULL AND einstempelzeit IS NOT NULL AND ausstempelzeit IS NOT NULL")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	type Arbeitszeit struct {
		Datum       string
		Arbeitszeit float64
	}
	var arbeitszeiten []Arbeitszeit
	//var arbeitszeiten []Arbeitszeit

	for rows.Next() {
		var einstempelzeit, ausstempelzeit, datum string

		err = rows.Scan(&datum, &einstempelzeit, &ausstempelzeit)
		if err != nil {
			log.Fatal(err)
		}
		// Parse time from string
		start, err := time.Parse("15:04", einstempelzeit)
		if err != nil {
			log.Fatal(err)
		}

		end, err := time.Parse("15:04", ausstempelzeit)
		if err != nil {
			log.Fatal(err)
		}

		// Calculate delta in seconds
		startSeconds := start.Hour()*3600 + start.Minute()*60 + start.Second()
		endSeconds := end.Hour()*3600 + end.Minute()*60 + end.Second()
		deltaSeconds := endSeconds - startSeconds

		// Convert delta to time.Duration in hours
		delta := time.Duration(deltaSeconds) * time.Second
		arbeitszeit := delta.Hours()

		arbeitszeit = math.Round(arbeitszeit*100) / 100
		arbeitszeiten = append(arbeitszeiten, Arbeitszeit{Datum: datum, Arbeitszeit: arbeitszeit})
	}

	for _, a := range arbeitszeiten {
		// Update arbeitszeit
		_, err = db.Exec("UPDATE stempelzeiten SET arbeitszeit = ? WHERE datum = ?", a.Arbeitszeit, a.Datum)
		if err != nil {
			log.Fatal("kann nicht schreiben: ", err)
		}
	}
}

func db_to_excel() {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT datum, einstempelzeit, ausstempelzeit, arbeitszeit FROM stempelzeiten WHERE eingetragen = 0 and arbeitszeit IS NOT NULL")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	type Stempelzeiten struct {
		Datum          string
		Einstempelzeit string
		Ausstempelzeit string
	}
	var stempelzeiten []Stempelzeiten

	for rows.Next() {
		var datum, einstempelzeit, ausstempelzeit string
		var arbeitszeit float64

		err = rows.Scan(&datum, &einstempelzeit, &ausstempelzeit, &arbeitszeit)
		if err != nil {
			log.Fatal(err)
		}
		if arbeitszeit > 6.0 {
			stempelzeiten = append(stempelzeiten, Stempelzeiten{Datum: datum, Einstempelzeit: einstempelzeit, Ausstempelzeit: "12:00"})
			stempelzeiten = append(stempelzeiten, Stempelzeiten{Datum: datum, Einstempelzeit: "12:30", Ausstempelzeit: ausstempelzeit})
		} else {
			stempelzeiten = append(stempelzeiten, Stempelzeiten{Datum: datum, Einstempelzeit: einstempelzeit, Ausstempelzeit: ausstempelzeit})
		}
	}
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	for i, s := range stempelzeiten {
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", i+1), "Mobiles Arbeiten")
		f.SetCellValue("Sheet1", fmt.Sprintf("B%d", i+1), s.Datum)
		f.SetCellValue("Sheet1", fmt.Sprintf("C%d", i+1), s.Datum)
		f.SetCellValue("Sheet1", fmt.Sprintf("D%d", i+1), s.Einstempelzeit)
		f.SetCellValue("Sheet1", fmt.Sprintf("E%d", i+1), s.Ausstempelzeit)
		f.SetCellValue("Sheet1", fmt.Sprintf("F%d", i+1), "1")
	}

	var firstDate, lastDate string
	err = db.QueryRow("SELECT datum FROM stempelzeiten WHERE eingetragen = 0 ORDER BY datum DESC LIMIT 1").Scan(&firstDate)
	if err != nil {
		log.Fatal(err)
	}

	err = db.QueryRow("SELECT datum FROM stempelzeiten WHERE eingetragen = 0 ORDER BY datum ASC LIMIT 1").Scan(&lastDate)
	if err != nil {
		log.Fatal(err)
	}
	// Save spreadsheet by the given path.
	filename := fmt.Sprintf("arbeitszeiten_%s_%s.xlsx", firstDate, lastDate)
	if err := f.SaveAs(filename); err != nil {
		fmt.Println(err)
	}
	for i := range stempelzeiten {
		_, err = db.Exec("UPDATE stempelzeiten SET eingetragen = 1 WHERE datum = ?", stempelzeiten[i].Datum)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func reset_arbeitszeit() {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("UPDATE stempelzeiten SET arbeitszeit = NULL WHERE arbeitszeit IS NOT NULL")
	if err != nil {
		log.Fatal(err)
	}
}

func displayQueryResult(label *widget.Label, query string) {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var results []string
	result := fmt.Sprintf("%s\t %s\t %s\t %s\t %s", "Datum", "Einstempelzeit", "Ausstempelzeit", "Arbeitszeit", "schon im Workon")
	results = append(results, result)
	for rows.Next() {
		var col1, col2, col3, col4, col5 string
		err = rows.Scan(&col1, &col2, &col3, &col4, &col5)
		if err != nil {
			log.Fatal(err)
		}
		result := fmt.Sprintf("%s\t %s\t\t\t %s\t\t\t %s\t\t %s", col1, col2, col3, col4, col5)
		results = append(results, result)
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	label.SetText(strings.Join(results, "\n"))
}

func get_last_einstempelzeit() string {
	db, err := sql.Open("sqlite3", "stunden.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	row := db.QueryRow("SELECT einstempelzeit FROM stempelzeiten WHERE ausstempelzeit IS NULL")
	var last_einstempelzeit string
	err = row.Scan(&last_einstempelzeit)
	if err != nil {
		return "schon eingestempelt?"
	}

	// Add 7 hours and 30 minutes to einstempelzeit
	einstempelzeit, err := time.Parse("15:04", last_einstempelzeit)
	if err != nil {
		log.Fatal(err)
	}
	newEinstempelzeit := einstempelzeit.Add(7*time.Hour + 30*time.Minute)

	return "Ende: " + newEinstempelzeit.Format("15:04")
}

func main() {

	createDatabaseAndTable()
	a := app.New()
	w := a.NewWindow("Time Tracker")
	Arbeitszeit_ende_label := widget.NewLabel(get_last_einstempelzeit())
	versionLabel := widget.NewLabel(fmt.Sprintf("Version: %s", Version))

	einstempelnButton := widget.NewButton("Einstempeln", func() {
		addDateIfNotExists()
		fmt.Println("Einstempeln button clicked")
		writeCurrentTime("einstempelzeit")
		Arbeitszeit_ende_label.SetText(get_last_einstempelzeit())
	})

	ausstempelnButton := widget.NewButton("Ausstempeln", func() {
		fmt.Println("Ausstempeln button clicked")
		writeCurrentTime("ausstempelzeit")
		calculateArbeitszeit()
	})

	eintragenButton := widget.NewButton("Excelfile mit Zeiten erstellen", func() {
		fmt.Println("Eintragen button clicked")
		db_to_excel()

	})

	reset_worktime_calculation_button := widget.NewButton("Debug only: Arbeitszeit neu berechnen", func() {
		fmt.Println("reset button clicked")
		reset_arbeitszeit()
		calculateArbeitszeit()

	})

	label := widget.NewLabel("")
	query_worktimes_button := widget.NewButton("Nicht eingetragene Zeiten anzeigen", func() {
		fmt.Println("Query button clicked")
		displayQueryResult(label, "SELECT * FROM stempelzeiten WHERE eingetragen=0")
	})

	openFile := func() {
		err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "stunden.db").Start()
		if err != nil {
			panic(err)
		}
	}
	open_db_outside_buttion := widget.NewButton("Debug only: DB öffnen (sqlite browser)", openFile)
	arbeitszeiten_anzeige_label := container.NewVScroll(label)
	arbeitszeiten_anzeige_label.SetMinSize(fyne.NewSize(600, 200))
	w.SetContent(container.NewVBox(
		einstempelnButton,
		ausstempelnButton,
		Arbeitszeit_ende_label,
		eintragenButton,
		reset_worktime_calculation_button,

		open_db_outside_buttion,
		query_worktimes_button,      // Add the query button to the window
		arbeitszeiten_anzeige_label, // Add the label to the window
		versionLabel,
	))
	//icon, err := fyne.LoadResourceFromPath("C:\\Users\\lum2do\\OneDrive - Bosch Group\\Stempeluhr\\clock-icon.png")
	icon, err := fyne.LoadResourceFromPath("clock-icon.png")
	if err != nil {
		fmt.Println(err)
	} else {
		w.SetIcon(icon)
	}

	w.ShowAndRun()
}
