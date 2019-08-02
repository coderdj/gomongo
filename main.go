package main

import (
	"context"
	"os"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"errors"
	"strconv"
	"io/ioutil"
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


func GetCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	detector := params["detector"]

	control_doc, err := GetControlDoc(detector)
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
		response.Write([]byte(`{"message": "Sorry, we don't support detector` +
			detector + ` yet!"}`))
		return
	}

	// As a precursor to doing anything the TPC DAQ must be IDLE and the current command
	// must have it 'deactivated'. So let's have a look then. First the control doc.
	control_doc, err := GetControlDoc(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "` + err.Error() + `"}`))
		return
	}
	if(control_doc.Active != "false" || control_doc.LinkMV != "false" ||
		control_doc.LinkNV != "false"){
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "TPC must be inactive and unlinked to other ` +
				` detectors to control via API"}`))
		return
	}
	// Now the status
	detector_status, err := GetDetectorStatus(detector, -1)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "`+err.Error()+`"}`))
		return
	}
	if detector_status.Status != 0 {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Detector ` + detector +
			` must be IDLE (0) but it is ` +
			fmt.Sprint(detector_status.Status) + `"}`))
		return
	}

	// The prerequisites are met, so now we can validate the incoming command
	// to make sure it's got everything it needs
	reqBody, err := ioutil.ReadAll(request.Body)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Error reading incoming request"}`))
		return
	}
	var new_control Control
	err = json.Unmarshal(reqBody, new_control)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Incoming request can't be unmarshalled"}`))
		return
	}

	// Do update
	err = UpdateControlDoc(new_control, request.FormValue("api_user"), detector);
	if err != nil {
		response.WriteHeader(http.StatusOK)
		response.Write([]byte(`{"message": "Update success!"}`))
		return
	}
	response.WriteHeader(http.StatusInternalServerError)
	response.Write([]byte(`{"message": "Error updating mongo: `+err.Error()+`"}`))
	return
}

func UpdateControlDoc(new_control Control, user string, detector string) (error){
	// Case 1: set inactive. Ignore everything else and just update to inactive.
	// Case 2: set active. Allow the following fields: (mode, stop_after, comment)
	//         And fix the following: LinkMV(false), LinkNV(false)
	//         Check options DB for 'mode'
	// All cases: set User to requesting API user
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	
	if new_control.Active == "false" {
		_, err := collection.UpdateOne(
			context.Background(),
			bson.M{"detector": detector},
			bson.M{"$set": bson.M{"active": "false", "user": user}},
		)
		return err
	}

	// Look up options
	options_collection := client.Database("daq").Collection("options")
	cursor, err := options_collection.Find(ctx, bson.M{"name": new_control.Mode})
	if err != nil{
		return err
	}
	// There is no 'count'. Amazing driver.
	cursorempty := true
	for cursor.Next(ctx){
		cursorempty = false
	}
	if cursorempty {
		return errors.New("There is no options doc by the name " + new_control.Mode)
	}

	// Probably need some check if Comment and StopAfter are included here.

	// Now we can update to start the DAQ
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"detector": detector},
		bson.M{"$set": bson.M{"active": "true", "mode": new_control.Mode,
			"user": user, "link_nv": "false", "link_mv": "false",
			"comment": new_control.Comment, "stop_after": new_control.StopAfter}},
	)
	if err != nil{
		return err
	}
	return nil
}

func GetControlDoc(detector string) (Control, error){
	// Just fetches the control doc for this detector
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector})
	var control_doc Control
	if err != nil {
		return control_doc, err
	}
	for cursor.Next(ctx) {
		cursor.Decode(&control_doc)
		return control_doc, nil;
	}
	return control_doc, errors.New("No control document found for detector " + detector);
}

func GetDetectorStatus(detector string, timeout int64) (DetectorStatus, error){
	// Gets the most recent detector status for detector. Optionally, timeout
	// ensures that the status is not stale past $timeout seconds or it will
	// set the error instead.
	collection := client.Database("daq").Collection("aggregate_status")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	options := options.Find()	
	options.SetSort(bson.D{{"_id", -1}})
	options.SetLimit(1)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector}, options)

	var aggregate_status DetectorStatus
	if err != nil {
		return aggregate_status, err
	}

	for cursor.Next(ctx) {
		
		cursor.Decode(&aggregate_status)

		// If timeout was given a value then check
		if timeout > 0 && time.Now().Unix()-aggregate_status.Time.Unix() > timeout{
			return aggregate_status, errors.New("Status doc found, but it's " +
				strconv.FormatInt(time.Now().Unix()-aggregate_status.Time.Unix(),10) +
				" seconds old");						
		}
		return aggregate_status, nil
	}
	return aggregate_status, errors.New("No status doc found at all for detector " + detector);
}
