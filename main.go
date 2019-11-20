package main

import (
	"context"
	"os"
	"fmt"
	"net/http"
	"time"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var client *mongo.Client
var runs_client *mongo.Client

func main() {
	fmt.Println("Starting the API")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	clientOptions := options.Client().ApplyURI(os.Getenv("DAQ_MONGO_URI"))	
	client, _ = mongo.Connect(ctx, clientOptions)
	runsClientOptions := options.Client().ApplyURI(os.Getenv("RUNS_MONGO_URI"))
	rctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	runs_client, _ = mongo.Connect(rctx, runsClientOptions);
	router := mux.NewRouter()
	router.HandleFunc("/helloworld", AuthCheck(HelloWorld)).Methods("GET")
	router.HandleFunc("/getcommand/{detector}", AuthCheck(GetCommandEndpoint)).Methods("GET")
	router.HandleFunc("/setcommand/{detector}", AuthCheck(UpdateCommandEndpoint)).Methods("POST")
	router.HandleFunc("/detector_status/{detector}",
		AuthCheck(GetDetectorStatusEndpoint)).Methods("GET")
	router.HandleFunc("/geterrors", AuthCheck(GetErrorsEndpoint)).Methods("GET")
	router.HandleFunc("/getstatus/{host}", AuthCheck(GetStatusEndpoint)).Methods("GET")
	http.ListenAndServe(":12345", router)
}

func AuthCheck(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {

		fmt.Println(request.URL.Path)
		// Get auth params from URL
		user := request.FormValue("api_user")
		key := request.FormValue("api_key")
		if key == "" || user == ""{
			response.WriteHeader(http.StatusInternalServerError)
			response.Write([]byte(`{"message": "Access denied"}`))
			return
		}
		// Check database for user
		collection := runs_client.Database("run").Collection("users")
		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		cursor, err := collection.Find(ctx, bson.M{"api_username": user})
		if err != nil {
			response.WriteHeader(http.StatusInternalServerError)
			response.Write([]byte(`{"message": "` + err.Error() + `"}`))
			return
		}
		// Package as json and return
		for cursor.Next(ctx) {
			var user_doc User
			cursor.Decode(&user_doc)
			if bcrypt.CompareHashAndPassword([]byte(user_doc.APIKey), []byte(key)) == nil{
				handlerFunc(response,request)
				return
			}
			break
		}
		// If it fails, then do this
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Access denied"}`))
		return
	}
}

func HelloWorld(response http.ResponseWriter, request *http.Request){
	/*
              Basic check to see if you can see the API. Should function even
              if the backend database is down.
        */

	t := time.Now().UTC()
	tstring := t.Format("2006-01-02 15:04:05")
	
	response.Header().Set("content-type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write([]byte(fmt.Sprintf(`{"message": "Hello to you too. The current time is %s"}`,
		tstring)))
	return
}
