package main

import (
	"context"
	"os"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"strconv"
	"errors"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var client *mongo.Client

func main() {
	fmt.Println("Starting the API")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	clientOptions := options.Client().ApplyURI(os.Getenv("DAQ_MONGO_URI"))
	client, _ = mongo.Connect(ctx, clientOptions)
	router := mux.NewRouter()
	router.HandleFunc("/helloworld", AuthCheck(HelloWorld)).Methods("GET")
	router.HandleFunc("/getcommand/{detector}", AuthCheck(GetCommandEndpoint)).Methods("GET")
	router.HandleFunc("/setcommand/{detector}", AuthCheck(UpdateCommandEndpoint)).Methods("POST")
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
		collection := client.Database("daq").Collection("users")
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

func GetStatusEndpoint(response http.ResponseWriter, request *http.Request) {
	/*
               Provides endpoint /status/{host}, where {host} is the hostname
               of the node you're querying. By default provides only the most 
               recent status doc from the host, but can also provide documents
               for the last 'n' seconds with argument time_seconds=n, i.e.
               /status/{host}?time_seconds=3600 for the last hour.
        */
	
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	host := params["host"]
	time_seconds, err := strconv.Atoi(request.FormValue("time_seconds"))
	if err != nil{
		time_seconds = 0
	}

	// Mongo connectivity and context
	collection := client.Database("daq").Collection("status")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	// Set search options and make query
	options := options.Find()
	options.SetSort(bson.D{{"_id", -1}})
	query := bson.M{"host": host}
	if time_seconds > 0 {
		hex_time := fmt.Sprintf("%X", time.Now().Unix() - int64(time_seconds))
		query_oid, _ := primitive.ObjectIDFromHex(hex_time + "0000000000000000")
		query = bson.M{"host": host, "_id": bson.M{"$gte": query_oid}}
	} else {
		options.SetLimit(1)
	}
	cursor, err := collection.Find(ctx, query, options)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}

	// Package as json and return
	var statii []Status
	for cursor.Next(ctx) {
		var status Status
		cursor.Decode(&status)
		if time_seconds == 0 {
			json.NewEncoder(response).Encode(status)
			return
		} else {
			statii = append(statii, status);
		}
	}
	if len(statii) > 0 {
		json.NewEncoder(response).Encode(statii)
		return
	}
	
	// If we hit this point then the cursor was empty
	response.WriteHeader(http.StatusInternalServerError)
	response.Write([]byte(`{"message": "query returned no documents"}`))
	return
}


func GetCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	detector := params["detector"]

	control_doc, err = GetControlDoc(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}
	json.NewEncoder(response).Encode(control_doc)
	return		
}

func UpdateCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	if err := request.ParseForm(); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Malformed request error: ` + err.Error() + `"}`))
		return
	}

	// Right now we ONLY support controlling the TPC. Fail if not TPC.
	// Probably if you're reading this you want to make it support the other detectors,
	// so right here is a good place to start. :-)
	params := mux.Vars(request)
	detector := params["detector"]
	if detector != "tpc" {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Sorry, we don't support detector`+
			detector+` yet!"}`))
		return
	}

	// As a precursor to doing anything the TPC DAQ must be IDLE and the current command
	// must have it 'deactivated'. So let's have a look then. First the control doc.
	control_doc, err := GetControlDoc(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "`+err.Error()+`"}`))
		return
	}
	if(control_doc.Active != "false" || control_doc.link_mv != "false" ||
		control_doc.link_nv != "false"){
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "TPC must be inactive and unlinked to other ` +
				` detectors to control via API"}`))
		return
	}
	// Now the status
	detector_status, err := GetDetectorStatus(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "`+err.Error()+`"}`))
		return
	}

	
}

func GetControlDoc(detector) (Control, error){
	// Just fetches the control doc for this detector
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector})
	if err != nil {
		return nil, err
	}
	for cursor.Next(ctx) {
		var control_doc Control
		cursor.Decode(&control_doc)
		return control_doc, nil;
	}
	return nil, errors.New("No control document found for detector " + detector);
}

func GetDetectorStatus(detector, timeout) (DetectorStatus, error){
	// Gets the most recent detector status for detector. Optionally, timeout
	// ensures that the status is not stale past $timeout seconds or it will
	// set the error instead.
	collection := client.Database("daq").Collection("aggregate_status")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	options := options.Find()	
	options.SetSort(bson.D{{"_id", -1}})
	options.SetLimit(1)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector}, options)
	if err != nil {
		return nil, err
	}

	for cursor.Next(ctx) {
		var aggregate_status DetectorStatus
		cursor.Decode(&aggregate_status)

		// If timeout was given a value then check
		if timeout > 0 && time.Now().Unix()-aggregate_status.Unix() > timeout{
			return nil, errors.New("Status doc found, but it's " +
				(time.Now().Unix()-aggregate_status.Unix()).toString() +
				" seconds old");						
		}
		return aggregate_status, nil
	}
	return nil, errors.New("No status doc found at all for detector " + detector);
}
