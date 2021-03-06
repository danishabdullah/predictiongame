package predictiongame

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/pborman/uuid"
)

// NumQuestions contains the number of questions for a single round.
const NumQuestions = 12

// ExpectedConfidence is the confidence that is expected from the user.
const ExpectedConfidence = 0.5

func initHandlers(mux *http.ServeMux, templ *template.Template, questions QuestionDatabase, games GameDatabase) {
	mux.Handle("/api/questions/random", questionHandler(questions))

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	mux.Handle("/play/", playHandler(templ, questions))
	mux.Handle("/play", newGameHandler())
	mux.Handle("/game/", gameHandler(templ, games))
	mux.Handle("/game", submitHandler(games))
	mux.Handle("/lastGame/", lastGameHandler(games))
	mux.Handle("/about", simpleHandler(templ, "about.html"))
	mux.Handle("/help/overview", simpleHandler(templ, "help-overview.html"))
	mux.Handle("/help/elements", simpleHandler(templ, "help-elements.html"))
	mux.Handle("/", simpleHandler(templ, "index.html"))
}

func render(templ *template.Template, w io.Writer, name string, value interface{}) {
	if err := templ.ExecuteTemplate(w, name, value); err != nil {
		log.Printf("Error rendering template: %s", err)
	}
}

func simpleHandler(templ *template.Template, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		render(templ, w, name, nil)
	})
}

func newGameHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewRandom().String()
		// TODO: save game id somewhere

		http.Redirect(w, r, fmt.Sprintf("/play/%s", id), http.StatusFound)
	})
}

type playContext struct {
	ID        string
	Questions []Question
}

func playHandler(templ *template.Template, db QuestionDatabase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)
		selected := db.SelectRandom(NumQuestions)

		render(templ, w, "play.html", playContext{
			ID:        id,
			Questions: selected,
		})
	})
}

func questionHandler(db QuestionDatabase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selected := db.SelectRandom(NumQuestions)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(selected); err != nil {
			log.Printf("Error writing JSON: %s", err)
		}
	})
}

// Answer contains the information about an answer given by the user.
type Answer struct {
	Question   Question `json:"question"`
	LowerBound float64  `json:"lower"`
	UpperBound float64  `json:"upper"`
}

// Correct returns true if the range given in the answer was correct.
func (a Answer) Correct() bool {
	qLow := a.Question.BoundLow
	qHigh := a.Question.BoundHigh
	aLow := a.LowerBound
	aHigh := a.UpperBound
	return (aLow >= qLow && aHigh <= qHigh) ||
		(aLow <= qLow && aHigh >= qLow) ||
		(aLow <= qHigh && aHigh >= qHigh)
}

func submitHandler(db GameDatabase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		defer r.Body.Close()

		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading request: %s", err), http.StatusBadRequest)
			return
		}

		raw := strings.TrimPrefix(string(bytes), "data=")
		data, err := url.QueryUnescape(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error decoding request: %s", err), http.StatusBadRequest)
			return
		}

		var game GameEntity
		if err := json.Unmarshal([]byte(data), &game); err != nil {
			http.Error(w, fmt.Sprintf("Error parsing answers: %s", err), http.StatusBadRequest)
			return
		}

		if err := db.Save(r, game.UserID, game.ID, game.Answers); err != nil {
			http.Error(w, fmt.Sprintf("Error saving game: %s", err), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/game/%s", game.ID), http.StatusFound)
	})
}

func gameHandler(templ *template.Template, db GameDatabase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)

		if len(id) == 0 {
			http.Redirect(w, r, "/", http.StatusFound)
		}

		game, err := db.Get(r, id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Game can not be loaded: %s", err), http.StatusNotFound)
			return
		}

		history, err := db.List(r, game.UserID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Game list can not be loaded: %s", err), http.StatusInternalServerError)
			return
		}

		render(templ, w, "game.html", struct {
			ID      string
			Answers []Answer
			History []GameEntity
		}{
			ID:      id,
			Answers: game.Answers,
			History: history,
		})
	})
}

func lastGameHandler(db GameDatabase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := path.Base(r.URL.Path)

		game, err := db.Last(r, uid)
		if err != nil {
			http.Error(w, fmt.Sprintf("Game list can not be loaded: %s", err), http.StatusInternalServerError)
			return
		}

		if game == nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/game/%s", game.ID), http.StatusFound)
	})
}
