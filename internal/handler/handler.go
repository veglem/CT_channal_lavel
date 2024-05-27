package http

import (
	"bytes"
	"chanalLevel/internal/usecase"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"math/rand"
	"net/http"
)

const frameLoseProbability = 16  // проценты * 10
const frameErrorProbability = 10 // проценты

const transferEndpoint = "http://192.168.244.111:8080/transfer"

type CodeRequest struct {
	Id        int    `json:"id"`
	MessageId int    `json:"message_id"`
	Sender    string `json:"login"`
	Time      int    `json:"timestamp"`
	SegCount  uint32 `json:"segments_count"`
	SegNum    uint32 `json:"segment_number"`
	Payload   string `json:"data"`
	Error     string `json:"error"`
}

type CodeTransferRequest struct {
	Id        int    `json:"id"`
	MessageId int    `json:"message_id"`
	Sender    string `json:"sender"`
	Time      int    `json:"timestamp"`
	SegCount  uint32 `json:"segments_count"`
	SegNum    uint32 `json:"segment_number"`
	Payload   string `json:"data"`
	Error     string `json:"error"`
}

// Code
// @Summary		Code network flow
// @Tags		Code
// @Description	Осуществляет кодировку сообщения в код Хэмминга [15, 11], внесение ошибки в каждый закодированный 15-битовый кадр с вероятностью 10%, исправление внесённых ошибок, раскодировку кадров в изначальное сообщение. Затем отправляет результат в Procuder-сервис транспортного уровня. Сообщение может быть потеряно с вероятностью 1%.
// @Accept		json
// @Produce     json
// @Param		request		body		CodeRequest		true	"Информация о сегменте сообщения"
// @Success		200			{object}	nil					"Обработка и отправка запущены"
// @Failure		400			{object}	nil					"Ошибка при чтении сообщения"
// @Router		/code [post]
func Code(c *gin.Context) {
	var codeRequest CodeRequest
	if err := c.Bind(&codeRequest); err != nil {
		c.Data(http.StatusBadRequest, c.ContentType(), []byte("Can't read request body:"+err.Error()))
		return
	}

	go transfer(codeRequest)

	c.Data(http.StatusOK, c.ContentType(), []byte{})
}

func transfer(codeRequest CodeRequest) {
	if rand.Intn(1000) < frameLoseProbability {
		fmt.Println("[Info] Message lost")
		return
	}

	processedMessage, hasErrors, err := processMessage([]byte(codeRequest.Payload))
	if err != nil {
		fmt.Printf("[Error] Error while processing message: %s\n", err.Error())
		return
	}

	// Отправка данных в Producer
	transferReqBody, err := json.Marshal(
		CodeTransferRequest{
			Id:        codeRequest.Id,
			MessageId: codeRequest.MessageId,
			Sender:    codeRequest.Sender,
			Time:      codeRequest.Time,
			SegCount:  codeRequest.SegCount,
			SegNum:    codeRequest.SegNum,
			Payload:   string(processedMessage),
			Error:     hasErrors,
		},
	)

	fmt.Printf("[Info] Prepared message: %s\n", CodeTransferRequest{
		Id:        codeRequest.Id,
		MessageId: codeRequest.MessageId,
		Sender:    codeRequest.Sender,
		Time:      codeRequest.Time,
		SegCount:  codeRequest.SegCount,
		SegNum:    codeRequest.SegNum,
		Payload:   string(processedMessage),
		Error:     hasErrors,
	}.Payload)

	if err != nil {
		fmt.Printf("[Error] Can't create transfer request: %s\n", err.Error())
		return
	}

	req, err := http.NewRequest("POST", transferEndpoint, bytes.NewBuffer(transferReqBody))
	if err != nil {
		fmt.Printf("[Error] Can't create transfer request: %s\n", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[Error] Transfer request issue: %s\n", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[Info] Unexpected status code while transferring %d\n", resp.StatusCode)
	}
}

// Возвращает декодированные биты (в случае успеха), флаг ошибки декодирования
// и ошибку исполнения при наличии
func processMessage(message []byte) ([]byte, string, error) {
	coder := usecase.Coder{}

	// Кодирование полученных данных
	encodedFrames, err := coder.Encode(message)
	if err != nil {
		log.Println("[Error] Encoding issue")
		return nil, "", errors.New(fmt.Sprintf("Enocding issue: %s", err.Error()))
	}

	// Внесение ошибок в закодированные кадры
	encodedFrames = coder.SetRandomErrors(encodedFrames, frameErrorProbability)

	// Исправление ошибок и декодирование кадров
	decodedFrames, err := coder.FixAndDecode(encodedFrames)
	if err != nil {
		log.Println("[Error] Decoding issue")
		return nil, "", errors.New(fmt.Sprintf("Decoding issue: %s", err.Error()))
	}

	// Валидация совпадения пришедших данных и выходных
	hasError := ""
	for ind, _byte := range message {
		if _byte != decodedFrames[ind] {
			log.Println("[Error] Frames inequality")
			hasError = "Error"
			break
		}
	}

	return decodedFrames, hasError, nil
}
