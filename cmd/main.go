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
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Defines a "model" that we can use to communicate with the
// frontend or the database
type BookStore struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	BookName   string
	BookAuthor string
	BookISBN   string
	BookPages  int
	BookYear   int
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
			BookName:   "The Vortex",
			BookAuthor: "José Eustasio Rivera",
			BookISBN:   "958-30-0804-4",
			BookPages:  292,
			BookYear:   1924,
		},
		{
			BookName:   "Frankenstein",
			BookAuthor: "Mary Shelley",
			BookISBN:   "978-3-649-64609-9",
			BookPages:  280,
			BookYear:   1818,
		},
		{
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
			"ID":         res.ID.Hex(),
			"BookName":   res.BookName,
			"BookAuthor": res.BookAuthor,
			"BookISBN":   res.BookISBN,
			"BookPages":  res.BookPages,
			"BookYear":   res.BookYear,
		})
	}

	return ret
}

func findAuthors(coll *mongo.Collection) []map[string]interface{} {
	cursor, err := coll.Find(context.TODO(), bson.D{{}})
	if err != nil {
		panic(err)
	}

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

	var ret []map[string]interface{}
	for author, books := range authorBooks {
		ret = append(ret, map[string]interface{}{
			"BookName":   books,
			"BookAuthor": author,
		})
	}
	return ret
}

func findYears(coll *mongo.Collection) []map[string]interface{} {
	cursor, err := coll.Find(context.TODO(), bson.D{{}})
	if err != nil {
		panic(err)
	}

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

	var ret []map[string]interface{}
	for year, books := range yearBooks {
		ret = append(ret, map[string]interface{}{
			"BookYear": year,
			"BookName": books,
		})
	}
	return ret
}

func main() {
	// Connect to the database. Such defer keywords are used once the local
	// context returns; for this case, the local context is the main function
	// By user defer function, we make sure we don't leave connections
	// dangling despite the program crashing. Isn't this nice? :D

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	uri := os.Getenv("MONGODB_URI")
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
		authors := findAuthors(coll)
		return c.Render(http.StatusOK, "author-table", authors)
	})

	e.GET("/years", func(c echo.Context) error {
		years := findYears(coll)
		return c.Render(http.StatusOK, "year-table", years)
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

		if len(results) == 0 {
			return c.String(http.StatusNotFound, "Entry not found")
		}

		var books []map[string]interface{}
		for _, res := range results {
			books = append(books, map[string]interface{}{
				"ID":         res.ID.Hex(),
				"BookName":   res.BookName,
				"BookAuthor": res.BookAuthor,
				"BookISBN":   res.BookISBN,
				"BookPages":  res.BookPages,
				"BookYear":   res.BookYear,
			})
		}
		return c.Render(http.StatusOK, "book-table", books)
	})

	e.POST("/api/books", func(c echo.Context) error {
		var book BookStore
		book.BookName = c.FormValue("BookName")
		book.BookAuthor = c.FormValue("BookAuthor")
		book.BookISBN = c.FormValue("BookISBN")

		pages := c.FormValue("BookPages")
		year := c.FormValue("BookYear")

		if pages != "" {
			if p, err := strconv.Atoi(pages); err == nil {
				book.BookPages = p
			}
		}
		if year != "" {
			if y, err := strconv.Atoi(year); err == nil {
				book.BookYear = y
			}
		}

		if book.BookName == "" {
			return c.String(http.StatusBadRequest, "Missing required fields: BookName")
		}

		filter := bson.M{"bookname": book.BookName, "bookisbn": book.BookISBN}
		fmt.Println("Received book name:", book.BookName)

		count, err := coll.CountDocuments(context.TODO(), filter)
		if err != nil {
			return c.String(http.StatusInternalServerError, "DB error")
		}
		fmt.Println("Duplicate count for book name:", count)
		if count > 0 {
			return c.String(http.StatusConflict, "Duplicate entry")
		}

		result, err := coll.InsertOne(context.TODO(), book)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Insert error")
		}
		fmt.Printf("inserted_id: %v\n", result.InsertedID)
		return c.Render(http.StatusOK, "index", nil)

	})

	e.PUT("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return c.String(http.StatusBadRequest, "Invalid book ID")
		}

		var book BookStore
		if err := c.Bind(&book); err != nil {
			return c.String(http.StatusBadRequest, "Invalid request")
		}

		book.BookName = strings.TrimSpace(book.BookName)
		book.BookAuthor = strings.TrimSpace(book.BookAuthor)
		book.BookISBN = strings.TrimSpace(book.BookISBN)

		if book.ID != primitive.NilObjectID && book.ID != objID {
			return c.String(http.StatusBadRequest, "Book ID in body doesn't match URL parameter")
		}

		filter := bson.M{"_id": objID}
		var existingBook BookStore
		err = coll.FindOne(context.TODO(), filter).Decode(&existingBook)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return c.String(http.StatusNotFound, "Book not found")
			}
			return c.String(http.StatusInternalServerError, "Database error")
		}

		fmt.Printf("Existing book: %+v\n", existingBook)
		fmt.Printf("Update request: %+v\n", book)

		if book.BookName != "" && book.BookName != existingBook.BookName {
			nameFilter := bson.M{
				"bookname": book.BookName,
				"_id":      bson.M{"$ne": objID},
			}
			count, err := coll.CountDocuments(context.TODO(), nameFilter)
			if err != nil {
				fmt.Printf("Error checking BookName duplicates: %v\n", err)
				return c.String(http.StatusInternalServerError, "Database error")
			}
			fmt.Printf("Books with same name (excluding current): %d\n", count)
			if count > 0 {
				return c.String(http.StatusConflict, "Book name already exists")
			}
		}

		if book.BookISBN != "" && book.BookISBN != existingBook.BookISBN {
			isbnFilter := bson.M{
				"bookisbn": book.BookISBN,
				"_id":      bson.M{"$ne": objID},
			}
			count, err := coll.CountDocuments(context.TODO(), isbnFilter)
			if err != nil {
				fmt.Printf("Error checking BookISBN duplicates: %v\n", err)
				return c.String(http.StatusInternalServerError, "Database error")
			}
			fmt.Printf("Books with same ISBN (excluding current): %d\n", count)
			if count > 0 {
				return c.String(http.StatusConflict, "Book ISBN already exists")
			}
		}

		updateFields := bson.M{}
		if book.BookAuthor != "" {
			updateFields["BookAuthor"] = book.BookAuthor
		}
		if book.BookPages != 0 {
			updateFields["BookPages"] = book.BookPages
		}
		if book.BookYear != 0 {
			updateFields["BookYear"] = book.BookYear
		}

		if len(updateFields) == 0 {
			return c.String(http.StatusBadRequest, "No fields to update")
		}

		fmt.Printf("Update fields: %+v\n", updateFields)

		update := bson.M{"$set": updateFields}
		res, err := coll.UpdateOne(context.TODO(), filter, update)
		if err != nil {
			fmt.Printf("Update error: %v\n", err)
			return c.String(http.StatusInternalServerError, "Update error: "+err.Error())
		}
		if res.MatchedCount == 0 {
			return c.String(http.StatusNotFound, "Book not found")
		}

		fmt.Printf("Successfully updated book. Matched: %d, Modified: %d\n", res.MatchedCount, res.ModifiedCount)
		return c.Render(http.StatusOK, "index", nil)
	})
	e.DELETE("/api/books/:id", func(c echo.Context) error {
		id := c.Param("id")
		objectID, err := primitive.ObjectIDFromHex(id)

		res, err := coll.DeleteOne(context.TODO(), bson.M{"_id": objectID})
		if err != nil {
			return c.String(http.StatusInternalServerError, "Delete error")
		}
		if res.DeletedCount == 0 {
			return c.String(http.StatusNotFound, "Book not found")
		}

		return c.Render(http.StatusOK, "index", nil)
	})

	e.Logger.Fatal(e.Start(":3030"))
}
