package task

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var DateFormat = "20060102"

type Task struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Comment string `json:"comment"`
	Repeat  string `json:"repeat"`
}

// AddTask добавляет новую задачу
func AddTask(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)
	if req.Method != http.MethodPost {
		http.Error(w, `{"error": "Метод не разрешен"}`, http.StatusMethodNotAllowed)
		return
	}
	var task Task

	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&task); err != nil {
		log.Printf("Ошибка десериализации JSON")
		http.Error(w, `{"error": "Ошибка десериализации JSON"}`, http.StatusBadRequest)
		return
	}

	if task.Title == "" {
		log.Printf("Не указан заголовок задачи")
		http.Error(w, `{"error": "Не указан заголовок задачи"}`, http.StatusBadRequest)
		return
	}

	date, err := ParseDate(task.Date)
	if err != nil {
		log.Printf("Неправильный формат времени")
		http.Error(w, `{"error": "Неправильный формат времени"}`, http.StatusBadRequest)
		return
	}

	if task.Repeat != "" && !regexp.MustCompile(`^(d \d{1,3}|y)$`).MatchString(task.Repeat) {
		log.Printf("Неправильно указано правило повторения")
		http.Error(w, `{"error": "Неправильно указано правило повторения"}`, http.StatusBadRequest)
		return
	}

	var newDateStr string
	now := time.Now().Local().Truncate(24 * time.Hour)
	if date.Before(now) && task.Repeat == "" {
		date = now
	}
	if date.Before(now) && task.Repeat != "" {
		newDateStr, err = NextDate(now, task.Date, task.Repeat)
		if err != nil {
			log.Printf("Не удается вычислить новую дату")
			http.Error(w, `{"error": "Не удается вычислить новую дату"}`, http.StatusBadRequest)
			return
		}
	} else {
		newDateStr = date.Format(DateFormat)
	}

	task.Date = newDateStr

	res, err := db.Exec("INSERT INTO scheduler (date, title, comment, repeat) VALUES (?, ?, ?, ?)",
		task.Date, task.Title, task.Comment, task.Repeat)
	if err != nil {
		log.Printf("insert failed")
		http.Error(w, `{"error": "insert failed"}`, http.StatusInternalServerError)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("scan id failed")
		http.Error(w, `{"error": "scan id failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	response := map[string]interface{}{"id": id}
	json.NewEncoder(w).Encode(response)
}

// GetTasks возвращает список задач
func GetTasks(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)
	if req.Method != http.MethodGet {
		http.Error(w, `{"error": "Метод не разрешен"}`, http.StatusMethodNotAllowed)
		return
	}

	var tasks []Task
	limit := 10
	rows, err := db.Query(`SELECT id, date, title, comment, repeat FROM scheduler ORDER BY date LIMIT ?;`, limit)
	if err != nil {
		log.Printf("select failed")
		http.Error(w, `{"error": "select failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		t := Task{}
		err := rows.Scan(&t.ID, &t.Date, &t.Title, &t.Comment, &t.Repeat)
		if err != nil {
			log.Printf("scan failed")
			http.Error(w, `{"error": "scan failed"}`, http.StatusInternalServerError)
			return
		}
		if t.Date == "" {
			t.Date = time.Now().Format(DateFormat)
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		log.Printf("rows unpacking failed")
		http.Error(w, `{"error": "rows unpacking failed"}`, http.StatusInternalServerError)
		return
	}

	if len(tasks) < 1 {
		tasks = make([]Task, 0)
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string][]Task{"tasks": tasks}); err != nil {
		log.Printf("Ошибка при формировании ответа: %v\n", err)
		http.Error(w, `{"error": "Ошибка при формировании ответа: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
}

// GetTask возвращает одну задачу по ID
func GetTask(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)
	if req.Method != http.MethodGet {
		log.Printf("Метод не разрешен ")
		http.Error(w, `{"error": "Метод не разрешен"}`, http.StatusMethodNotAllowed)
		return
	}
	id := req.URL.Query().Get("id")

	if id == "" {
		log.Printf("Не указан идентификатор задачи")
		http.Error(w, `{"error": "Не указан идентификатор задачи"}`, http.StatusBadRequest)
		return
	}

	var t Task
	row := db.QueryRow("SELECT  id, date, title, comment, repeat FROM scheduler WHERE id = ?", id)
	err := row.Scan(&t.ID, &t.Date, &t.Title, &t.Comment, &t.Repeat)
	if err == sql.ErrNoRows {
		log.Printf("Задача (ID: %s): %v не найдена", id, err)
		http.Error(w, `{"error": "Задача не найдена"}`, http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Ошибка поиска задачи (ID: %s): %v", id, err)
		http.Error(w, `{"error": "Ошибка поиска задачи в БД"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if err := json.NewEncoder(w).Encode(t); err != nil {
		log.Printf("Ошибка при формировании ответа: %v\n", err)
		http.Error(w, `{"error": "Ошибка при формировании ответа: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
}

// UpdateTask обновляет существующую задачу
func UpdateTask(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)
	if req.Method != http.MethodPut {
		http.Error(w, `{"error": "Метод не разрешен"}`, http.StatusMethodNotAllowed)
		return
	}

	var task Task
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&task); err != nil {
		log.Printf("Ошибка десериализации JSON")
		http.Error(w, `{"error": "Ошибка десериализации JSON"}`, http.StatusBadRequest)
		return
	}

	if task.ID == "" {
		log.Printf("Не указан идентификатор")
		http.Error(w, `{"error": "Не указан идентификатор"}`, http.StatusBadRequest)
		return
	}
	_, err := strconv.Atoi(task.ID)
	if err != nil {
		log.Printf("Некорректный идентификатор")
		http.Error(w, `{"error": "Некорректный идентификатор"}`, http.StatusBadRequest)
		return
	}
	if task.Title == "" {
		log.Printf("Не указан заголовок задачи")
		http.Error(w, `{"error": "Не указан заголовок задачи"}`, http.StatusBadRequest)
		return
	}

	taskDate, err := time.Parse(DateFormat, task.Date)
	if err != nil {
		log.Printf("Неправильный формат даты")
		http.Error(w, `{"error": "Неправильный формат даты"}`, http.StatusBadRequest)
		return
	}

	if taskDate.Before(time.Now().Local().Truncate(24 * time.Hour)) {
		log.Printf("Дата не может быть в прошлом")
		http.Error(w, `{"error": "Дата не может быть в прошлом"}`, http.StatusBadRequest)
		return
	}
	if task.Repeat == "" {
		log.Printf("Неправильно указано правило повторения")
		http.Error(w, `{"error": "Неправильно указано правило повторения"}`, http.StatusBadRequest)
		return
	}
	if !regexp.MustCompile(`^(d \d{1,3}|y)$`).MatchString(task.Repeat) {
		log.Printf("Неправильно указано правило повторения")
		http.Error(w, `{"error": "Неправильно указано правило повторения"}`, http.StatusBadRequest)
		return
	}

	var exist bool
	err = db.QueryRow("SELECT 1 FROM scheduler WHERE id = ?", task.ID).Scan(&exist)
	if err != nil {
		log.Printf("Не удалось сделать запрос к БД")
		http.Error(w, `{"error": "Не удалось сделать запрос к БД"}`, http.StatusNotFound)
		return
	}
	if !exist {
		log.Printf("Задача не найдена")
		http.Error(w, `{"error": "Задача не найдена"}`, http.StatusNotFound)
		return
	}

	_, err = db.Exec("UPDATE scheduler SET date = ?, title = ?, comment = ?, repeat = ? WHERE id = ?", task.Date, task.Title, task.Comment, task.Repeat, task.ID)
	if err != nil {
		log.Printf("Не удалось обновить задачу")
		http.Error(w, `{"error": "Не удалось обновить задачу"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

// DoneTask помечает задачу как выполненную
func DoneTask(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)

	id := req.URL.Query().Get("id")
	if id == "" {
		log.Printf("Не указан идентификатор задачи")
		http.Error(w, `{"error": "Не указан идентификатор задачи"}`, http.StatusBadRequest)
		return
	}
	_, err := strconv.Atoi(id)
	if err != nil {
		http.Error(w, `{"error": "Неверный идентификатор задачи"}`, http.StatusBadRequest)
		return
	}

	var t Task
	row := db.QueryRow("SELECT  date, repeat FROM scheduler WHERE id = ?", id)
	err = row.Scan(&t.Date, &t.Repeat)
	if err == sql.ErrNoRows {
		log.Printf("Задача не найдена в БД")
		http.Error(w, `{"error": "Задача не найдена"}`, http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Ошибка запроса к БД")
		http.Error(w, `{"error": "Ошибка запроса к БД"}`, http.StatusInternalServerError)
		return
	}

	if t.Repeat == "" {
		_, err = db.Exec("DELETE FROM scheduler WHERE id = ?", id)
		if err != nil {
			log.Printf("Не удалось удалить задачу")
			http.Error(w, `{"error": "Не удалось удалить задачу"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Write([]byte("{}"))
		return
	}

	newDate, err := NextDate(time.Now(), t.Date, t.Repeat)
	if err != nil {
		log.Printf("Не удалось вычислить следующую дату")
		http.Error(w, `{"error": "Не удалось вычислить следующую дату"}`, http.StatusNotFound)
		return
	}

	_, err = db.Exec("UPDATE scheduler SET date = ? WHERE id = ?", newDate, id)
	if err != nil {
		http.Error(w, `{"error": "Не удалось обновить задачу"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write([]byte("{}"))
}

// DeleteTask удаляет задачу
func DeleteTask(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	log.Printf("Получен запрос: %s %s", req.Method, req.URL.Path)
	if req.Method != http.MethodDelete {
		http.Error(w, `{"error": "Метод не разрешен"}`, http.StatusMethodNotAllowed)
		return
	}
	id := req.URL.Query().Get("id")
	if id == "" {
		log.Printf("Не указан идентификатор задачи")
		http.Error(w, `{"error": "Не указан идентификатор задачи"}`, http.StatusBadRequest)
		return
	}

	result, err := db.Exec("DELETE FROM scheduler WHERE id = ?", id)
	if err != nil {
		log.Printf("Не удалось удалить задачу")
		http.Error(w, `{"error": "Не удалось удалить задачу"}`, http.StatusNotFound)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "Задача не найдена"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write([]byte("{}"))
}

// ParseDate парсит строку даты
func ParseDate(dateStr string) (time.Time, error) {
	if dateStr != "" {
		return time.Parse(DateFormat, dateStr)
	}
	return time.Now(), nil
}

// NextDate вычисляет следующую дату с учетом правила повторения
func NextDate(now time.Time, date string, repeat string) (string, error) {
	if repeat == "" {
		err := errors.New("не указано правило повторения")
		return "", err
	}

	dateForm, err := time.Parse(DateFormat, date)
	if err != nil {
		err := errors.New("указан неверный формат времени")
		return "", err
	}
	rules := strings.Split(repeat, " ")

	switch rules[0] {
	case "d":
		if len(rules) != 2 {
			err = errors.New("неподдерживаемый формат правила повторения")
			return "", err
		}
		days, err := strconv.Atoi(rules[1])
		if err != nil {
			err = errors.New("неподдерживаемый формат правила повторения")
			return "", err
		}

		if days > 400 || days < 1 {
			err = errors.New("недопустимое количество дней")
			return "", err
		}
		for {
			dateForm = dateForm.AddDate(0, 0, days)
			if dateForm.After(now.Local().Truncate(24 * time.Hour)) {
				break
			}
		}
		return dateForm.Format(DateFormat), nil

	case "y":
		for {
			dateForm = dateForm.AddDate(1, 0, 0)
			if dateForm.After(now) {
				break
			}
		}
		return dateForm.Format(DateFormat), nil

	default:
		err = errors.New("недопустимое правило повторения")
		return "", err
	}
}
