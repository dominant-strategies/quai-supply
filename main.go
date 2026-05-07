package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
)

const (
	mainnetRPC = "https://rpc.quai.network/cyprus1"
	testnetRPC = "https://orchard.rpc.quai.network/cyprus1"
)

func callRPC(rpcURL, method string, params []interface{}) ([]byte, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", rpcURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func callRPCBatch(rpcURL, method string, params []interface{}) ([]byte, error) {
	requestBody := []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"method":  method,
			"params":  params,
			"id":      1,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", rpcURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// handleMiningInfo creates a handler for the mining info endpoint
func handleMiningInfo(rpcURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for decimal query parameter
		decimalParam := r.URL.Query().Get("Decimal")
		useDecimal := decimalParam == "true" || decimalParam == "1"

		// Prepare the request body
		var params []interface{}
		if useDecimal {
			params = []interface{}{true}
		} else {
			params = []interface{}{}
		}

		requestBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "quai_getMiningInfo",
			"params":  params,
			"id":      1,
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			http.Error(w, "Error preparing request", http.StatusInternalServerError)
			return
		}

		// Create and configure HTTP request
		client := &http.Client{}
		req, err := http.NewRequest("POST", rpcURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			http.Error(w, "Error creating request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Error making request to upstream", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Read the response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}

		// Forward the response as-is
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}

// handleRewardsAnalytics creates a handler for the cumulative Quai rewards endpoint.
func handleRewardsAnalytics(rpcURL string) http.HandlerFunc {
	lastResponse := "0"

	return func(w http.ResponseWriter, r *http.Request) {
		replyWithValue := func(value string) {
			if r.URL.Query().Get("raw") == "true" {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(w, value)
				return
			}
			response := map[string]string{
				"result": value,
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				fmt.Println(w, "Error encoding JSON", http.StatusInternalServerError)
			}
		}

		body, err := callRPCBatch(rpcURL, "quai_rewardAnalytics", []interface{}{})
		if err != nil {
			fmt.Println(w, "Error making request to upstream", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		type RewardsResponse struct {
			Result struct {
				CumulativeQuaiMined string `json:"cumulativeQuaiMined"`
			} `json:"result"`
		}

		rewardsResp, err := decodeRewardsResponse(body)
		if err != nil {
			fmt.Println(w, "Error parsing upstream response", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}
		if rewardsResp.Result.CumulativeQuaiMined == "" {
			fmt.Println(w, "Missing cumulativeQuaiMined in upstream response", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		lastResponse = rewardsResp.Result.CumulativeQuaiMined
		replyWithValue(lastResponse)
	}
}

func decodeRewardsResponse(body []byte) (struct {
	Result struct {
		CumulativeQuaiMined string `json:"cumulativeQuaiMined"`
	} `json:"result"`
}, error) {
	var rewardsResp struct {
		Result struct {
			CumulativeQuaiMined string `json:"cumulativeQuaiMined"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &rewardsResp); err == nil && rewardsResp.Result.CumulativeQuaiMined != "" {
		return rewardsResp, nil
	}

	var batchResp []struct {
		Result struct {
			CumulativeQuaiMined string `json:"cumulativeQuaiMined"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return rewardsResp, err
	}
	if len(batchResp) == 0 {
		return rewardsResp, nil
	}
	rewardsResp.Result.CumulativeQuaiMined = batchResp[0].Result.CumulativeQuaiMined
	return rewardsResp, nil
}

func main() {

	// In case of some error just log and return the start value
	lastResponse := big.NewInt(480000000)

	// Mining info endpoints
	http.HandleFunc("/mininginfo", handleMiningInfo(mainnetRPC))
	http.HandleFunc("/testnetmininginfo", handleMiningInfo(testnetRPC))
	http.HandleFunc("/rewards", handleRewardsAnalytics(mainnetRPC))
	http.HandleFunc("/testnetrewards", handleRewardsAnalytics(testnetRPC))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		replyWithValue := func(supply *big.Int) {
			if r.URL.Query().Get("raw") == "true" {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(w, supply.String())
				return
			}
			response := map[string]string{
				"result": supply.String(),
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				fmt.Println(w, "Error encoding JSON", http.StatusInternalServerError)
			}
		}

		// Prepare the request body
		requestBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "quai_getSupplyAnalyticsForBlock",
			"params":  []string{"latest"},
			"id":      1,
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			fmt.Println(w, "Error preparing request", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		// Create and configure HTTP request
		client := &http.Client{}
		req, err := http.NewRequest("POST", mainnetRPC, bytes.NewBuffer(jsonBody))
		if err != nil {
			fmt.Println(w, "Error creating request", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println(w, "Error making request to upstream", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}
		defer resp.Body.Close()

		// Read and parse the response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(w, "Error reading response", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		// Define structure to unmarshal the nested response
		type SupplyResponse struct {
			Result struct {
				QuaiSupplyTotal string `json:"quaiSupplyTotal"`
			} `json:"result"`
		}

		var supplyResp SupplyResponse
		if err := json.Unmarshal(body, &supplyResp); err != nil {
			fmt.Println(w, "Error parsing upstream response", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		// Convert hex to big.Int
		quaiTotal := new(big.Int)
		// Remove "0x" prefix and convert from hex
		if _, ok := quaiTotal.SetString(supplyResp.Result.QuaiSupplyTotal[2:], 16); !ok {
			fmt.Println(w, "Error converting hex to integer", http.StatusInternalServerError)
			replyWithValue(lastResponse)
			return
		}

		quaiTotalInQuai := new(big.Int).Div(quaiTotal, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

		lastResponse = quaiTotalInQuai

		replyWithValue(quaiTotalInQuai)

	})

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
