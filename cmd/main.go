package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Defines a "model" that we can use to communicate with the
// frontend or the database
type BookStore struct {
	ID         string
	BookName   string
	BookAuthor string
	BookISBN   string
	BookPages  int
	BookYear   int
}

type BookRequest struct {
	ID      string `json:"id" form:"id"`
	Title   string `json:"title" form:"title"`
	Author  string `json:"author" form:"author"`
	Pages   string `json:"pages,omitempty" form:"pages, omitempty"`
	Edition string `json:"edition,omitempty" form:"edition, omitempty"`
	Year    string `json:"year,omitempty" form:"year, omitempty"`
}

type BookResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Pages   string `json:"pages"`
	Edition string `json:"edition"`
	Year    string `json:"year"`
}

var idCounter int64 = 1000000

func getNextID(coll *mongo.Collection) string {
	pipeline := []bson.M{
		{"$match": bson.M{"id": bson.M{"$exists": true}}},
		{"$sort": bson.M{"id": -1}},
		{"$limit": 1},
	}

	cursor, err := coll.Aggregate(context.TODO(), pipeline)
	if err == nil {
		var results []BookStore
		if err = cursor.All(context.TODO(), &results); err == nil && len(results) > 0 {
			if existingID, err := strconv.ParseInt(results[0].ID, 10, 64); err == nil {
				idCounter = existingID + 1
			}
		}
	}

	id := strconv.FormatInt(idCounter, 10)
	idCounter++
	return id
}

// Wraps the "Template" struct to associate a necessary method
// to determine the rendering procedure
type Template struct {
	tmpl *template.Template
}

// Preload the available templates for the view folder.
// This builds a local "database" of all available "blocks"
// to render upon request, i.e., replace the respective
// variable or expression.
// For more on templating, visit https://jinja.palletsprojects.com/en/3.0.x/templates/
// to get to know more about templating
// You can also read Golang's documentation on their templating
// https://pkg.go.dev/text/template
func loadTemplates() *Template {
	return &Template{
		tmpl: template.Must(template.ParseGlob("views/*.html")),
	}
}

// Method definition of the required "Render" to be passed for the Rendering
// engine.
// Contraire to method declaration, such syntax defines methods for a given
// struct. "Interfaces" and "structs" can have methods associated with it.
// The difference lies that interfaces declare methods whether struct only
// implement them, i.e., only define them. Such differentiation is important
// for a compiler to ensure types provide implementations of such methods.
func (t *Template) Render(w io.Writer, name string, data interface{}, ctx echo.Context) error {
	return t.tmpl.ExecuteTemplate(w, name, data)
}

// Here we make sure the connection to the database is correct and initial
// configurations exists. Otherwise, we create the proper database and collection
// we will store the data.
// To ensure correct management of the collection, we create a return a
// reference to the collection to always be used. Make sure if you create other
// files, that you pass the proper value to ensure communication with the
// database
// More on what bson means: https://www.mongodb.com/docs/drivers/go/current/fundamentals/bson/
func prepareDatabase(client *mongo.Client, dbName string, collecName string) (*mongo.Collection, error) {
	db := client.Database(dbName)

	names, err := db.ListCollectionNames(context.TODO(), bson.D{{}})
	if err != nil {
		return nil, err
	}
	if !slices.Contains(names, collecName) {
		cmd := bson.D{{"create", collecName}}
		var result bson.M
		if err = db.RunCommand(context.TODO(), cmd).Decode(&result); err != nil {
			log.Fatal(err)
			return nil, err
		}
	}

	coll := db.Collection(collecName)
	return coll, nil
}

// Here we prepare some fictional data and we insert it into the database
// the first time we connect to it. Otherwise, we check if it already exists.
func prepareData(client *mongo.Client, coll *mongo.Collection) {
	startData := []BookStore{
		{
			ID:         getNextID(coll),
			BookName:   "The Vortex",
			BookAuthor: "José Eustasio Rivera",
			BookISBN:   "958-30-0804-4",
			BookPages:  292,
			BookYear:   1924,
		},
		{
			ID:         getNextID(coll),
			BookName:   "Frankenstein",
			BookAuthor: "Mary Shelley",
			BookISBN:   "978-3-649-64609-9",
			BookPages:  280,
			BookYear:   1818,
		},
		{
			ID:         getNextID(coll),
			BookName:   "The Black Cat",
			BookAuthor: "Edgar Allan Poe",
			BookISBN:   "978-3-99168-238-7",
			BookPages:  280,
			BookYear:   1843,
		},
	}

	// This syntax helps us iterate over arrays. It behaves similar to Python
	// However, range always returns a tuple: (idx, elem). You can ignore the idx
	// by using _.
	// In the topic of function returns: sadly, there is no standard on return types from function. Most functions
	// return a tuple with (res, err), but this is not granted. Some functions
	// might return a ret value that includes res and the err, others might have
	// an out parameter.
	for _, book := range startData {
		cursor, err := coll.Find(context.TODO(), book)
		var results []BookStore
		if err = cursor.All(context.TODO(), &results); err != nil {
			panic(err)
		}
		if len(results) > 1 {
			log.Fatal("more records were found")
		} else if len(results) == 0 {
			result, err := coll.InsertOne(context.TODO(), book)
			if err != nil {
				panic(err)
			} else {
				fmt.Printf("%+v\n", result)
			}

		} else {
			for _, res := range results {
				cursor.Decode(&res)
				fmt.Printf("%+v\n", res)
			}
		}
	}
}

// Generic method to perform "SELECT * FROM BOOKS" (if this was SQL, which
// it is not :D ), and then we convert it into an array of map. In Golang, you
// define a map by writing map[<key type>]<value type>{<key>:<value>}.
// interface{} is a special type in Golang, basically a wildcard...
func findAllBooks(coll *mongo.Collection) []map[string]interface{} {
	cursor, err := coll.Find(context.TODO(), bson.D{{}})
	var results []BookStore
	if err = cursor.All(context.TODO(), &results); err != nil {
		panic(err)
	}

	var ret []map[string]interface{}
	for _, res := range results {
		ret = append(ret, map[string]interface{}{
			"ID":         res.ID,
			"BookName":   res.BookName,
			"BookAuthor": res.BookAuthor,
			"BookISBN":   res.BookISBN,
			"BookPages":  res.BookPages,
		})
	}

	return ret
}

func getBookByID(coll *mongo.Collection, id string) (map[string]interface{}, error) {
	var book BookStore
	err := coll.FindOne(context.TODO(), bson.M{"id": id}).Decode(&book)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"ID":         book.ID,
		"BookName":   book.BookName,
		"BookAuthor": book.BookAuthor,
		"BookISBN":   book.BookISBN,
		"BookPages":  book.BookPages,
		"BookYear":   book.BookYear,
	}, nil
}

func bookExists(coll *mongo.Collection, book BookStore) bool {
	filter := bson.M{
		"bookname":   book.BookName,
		"bookauthor": book.BookAuthor,
		"bookyear":   book.BookYear,
		"bookpages":  book.BookPages,
		"bookisbn":   book.BookISBN,
	}

	var result BookStore
	err := coll.FindOne(context.TODO(), filter).Decode(&result)
	return err == nil
}

func createBook(coll *mongo.Collection, bookReq BookRequest) error {
	pages := 0
	if bookReq.Pages != "" {
		if p, err := fmt.Sscanf(bookReq.Pages, "%d", &pages); err != nil || p != 1 {
			pages = 0
		}
	}

	year := 0
	if bookReq.Year != "" {
		if y, err := fmt.Sscanf(bookReq.Year, "%d", &year); err != nil || y != 1 {
			year = 0
		}
	}

	bookID := bookReq.ID
	if bookID == "" {
		bookID = getNextID(coll)
	}

	book := BookStore{
		ID:         bookID,
		BookName:   bookReq.Title,
		BookAuthor: bookReq.Author,
		BookISBN:   bookReq.Edition,
		BookPages:  pages,
		BookYear:   year,
	}

	if bookExists(coll, book) {
		return fmt.Errorf("book already exists")
	}

	_, err := coll.InsertOne(context.TODO(), book)
	return err
}

func updateBook(coll *mongo.Collection, id string, bookReq BookRequest) error {
	filter := bson.M{"id": id}

	pages := 0
	if bookReq.Pages != "" {
		if p, err := fmt.Sscanf(bookReq.Pages, "%d", &pages); err != nil || p != 1 {
			pages = 0
		}
	}

	year := 0
	if bookReq.Year != "" {
		if y, err := fmt.Sscanf(bookReq.Year, "%d", &year); err != nil || y != 1 {
			year = 0
		}
	}

	update := bson.M{"$set": bson.M{
		"bookname":   bookReq.Title,
		"bookauthor": bookReq.Author,
		"bookisbn":   bookReq.Edition,
		"bookpages":  pages,
		"bookyear":   year,
	}}

	result, err := coll.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("book not found")
	}

	return nil
}

func deleteBook(coll *mongo.Collection, id string) error {
	filter := bson.M{"id": id}
	result, err := coll.DeleteOne(context.TODO(), filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("book not found")
	}

	return nil
}

func getAuthors(coll *mongo.Collection) []map[string]interface{} {
	cursor, err := coll.Find(context.TODO(), bson.D{{}})
	var results []BookStore
	if err = cursor.All(context.TODO(), &results); err != nil {
		panic(err)
	}

	authorBooks := make(map[string][]string)

	for _, res := range results {
		if res.BookISBN == "" || res.BookName == "" {
			continue
		}
		authorBooks[res.BookAuthor] = append(authorBooks[res.BookAuthor], res.BookName)
	}

	var authors []map[string]interface{}
	for author, books := range authorBooks {
		authors = append(authors, map[string]interface{}{
			"BookName":   books,
			"BookAuthor": author,
		})
	}
	return authors
}

func getYears(coll *mongo.Collection) []map[string]interface{} {
	cursor, err := coll.Find(context.TODO(), bson.D{{}})
	var results []BookStore
	if err = cursor.All(context.TODO(), &results); err != nil {
		panic(err)
	}

	yearBooks := make(map[string][]string)
	for _, res := range results {
		if res.BookISBN == "" || res.BookName == "" {
			continue
		}
		var year = strconv.Itoa(res.BookYear)
		yearBooks[year] = append(yearBooks[year], res.BookName)
	}

	var years []map[string]interface{}
	for year, books := range yearBooks {
		years = append(years, map[string]interface{}{
			"BookYear": year,
			"BookName": books,
		})
	}
	return years
}

func main() {
	// Connect to the database. Such defer keywords are used once the local
	// context returns; for this case, the local context is the main function
	// By user defer function, we make sure we don't leave connections
	// dangling despite the program crashing. Isn't this nice? :D

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	uri := os.Getenv("DATABASE_URI")
	if len(uri) == 0 {
		fmt.Printf("failure to load env variable\n")
		os.Exit(1)
	}

	// TODO: make sure to pass the proper username, password, and port
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Printf("failed to create client for MongoDB\n")
		os.Exit(1)
	}

	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		fmt.Printf("failed to connect to MongoDB, please make sure the database is running\n")
		os.Exit(1)
	}

	// This is another way to specify the call of a function. You can define inline
	// functions (or anonymous functions, similar to the behavior in Python)
	defer func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()

	// You can use such name for the database and collection, or come up with
	// one by yourself!
	coll, err := prepareDatabase(client, "exercise-2", "information")

	// drop := coll.Drop(context.TODO())
	// if drop != nil {
	// 	log.Fatal("Failed to drop collection:", drop)
	// }

	prepareData(client, coll)

	// Here we prepare the server
	e := echo.New()

	// Define our custom renderer
	e.Renderer = loadTemplates()

	// Log the requests. Please have a look at echo's documentation on more
	// middleware
	e.Use(middleware.Logger())

	e.Static("/css", "css")

	// Endpoint definition. Here, we divided into two groups: top-level routes
	// starting with /, which usually serve webpages. For our RESTful endpoints,
	// we prefix the route with /api to indicate more information or resources
	// are available under such route.
	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index", nil)
	})

	e.GET("/books", func(c echo.Context) error {
		books := findAllBooks(coll)
		return c.Render(http.StatusOK, "book-table", books)
	})

	e.GET("/authors", func(c echo.Context) error {
		authors := getAuthors(coll)
		return c.Render(200, "author-table", authors)
	})

	e.GET("/years", func(c echo.Context) error {
		years := getYears(coll)
		return c.Render(200, "year-table", years)
	})

	e.GET("/search", func(c echo.Context) error {
		return c.Render(http.StatusOK, "search", nil)
	})

	e.GET("/create", func(c echo.Context) error {
		return c.Render(http.StatusOK, "create", nil)
	})

	e.GET("/api/books", func(c echo.Context) error {
		books := findAllBooks(coll)
		return c.JSON(http.StatusOK, books)
	})

	e.GET("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")
		book, err := getBookByID(coll, id)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
		}
		return c.JSON(http.StatusOK, book)
	})

	e.GET("/api/search", func(c echo.Context) error {
		query := c.QueryParam("q")
		fmt.Println("query:", query)
		if query == "" {
			return c.String(http.StatusBadRequest, "Missing search query")
		}

		filter := bson.M{
			"$or": []bson.M{
				{"bookname": bson.M{"$regex": query, "$options": "i"}},
				{"bookauthor": bson.M{"$regex": query, "$options": "i"}},
				{"bookisbn": bson.M{"$regex": query, "$options": "i"}},
				{"bookedition": bson.M{"$regex": query, "$options": "i"}},
			},
		}

		cursor, err := coll.Find(context.TODO(), filter)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Search error: "+err.Error())
		}
		var results []BookStore
		if err = cursor.All(context.TODO(), &results); err != nil {
			return c.String(http.StatusInternalServerError, "Cursor error: "+err.Error())
		}

		var books []map[string]interface{}
		for _, res := range results {
			books = append(books, map[string]interface{}{
				"id":     res.ID,
				"title":  res.BookName,
				"author": res.BookAuthor,
				"isbn":   res.BookISBN,
				"pages":  res.BookPages,
				"year":   res.BookYear,
			})
		}
		return c.Render(http.StatusOK, "book-table", books)
	})

	e.POST("/api/books", func(c echo.Context) error {
		var bookReq BookRequest

		if err := c.Bind(&bookReq); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if bookReq.Title == "" || bookReq.Author == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Title and author are required"})
		}

		if err := createBook(coll, bookReq); err != nil {
			if err.Error() == "book already exists" {
				return c.JSON(http.StatusConflict, map[string]string{"error": "Book already exists"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create book"})
		}

		return c.JSON(http.StatusCreated, map[string]string{"message": "Book created successfully"})
	})

	e.PUT("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")

		var bookReq BookRequest
		if err := c.Bind(&bookReq); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if bookReq.Title == "" || bookReq.Author == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Title and author are required"})
		}

		if err := updateBook(coll, id, bookReq); err != nil {
			if err.Error() == "book not found" {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update book"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Book updated successfully"})
	})

	e.DELETE("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")

		if err := deleteBook(coll, id); err != nil {
			if err.Error() == "book not found" {
				return c.NoContent(http.StatusNoContent)
			}
			return c.JSON(http.StatusOK, map[string]string{"error": "Failed to delete book"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Book deleted successfully"})
	})

	e.Logger.Fatal(e.Start(":3030"))
}
