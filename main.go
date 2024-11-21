package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	// 连接到以太坊主网 (Infura 也可以用，或是其他节点提供商)
	rpcURL := "https://lb.drpc.org/ogrpc?network=ethereum&dkey=Avduh2iIjEAksBUYtd4wP1NUPObEnwYR76WEFhW5UfFk"
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("无法连接到以太坊主网: %v", err)
	}
	defer client.Close()

	fmt.Println("正在监听以太坊新块...")

	// 使用轮询方式来获取新区块信息
	var lastBlockNumber uint64
	for {
		header, err := client.HeaderByNumber(context.Background(), nil)
		if err != nil {
			log.Printf("无法获取最新块头: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if header.Number.Uint64() > lastBlockNumber {
			lastBlockNumber = header.Number.Uint64()
			fmt.Printf("检测到新块，块号: %d, 块哈希: %s\n", header.Number.Uint64(), header.Hash().Hex())

			// 获取该块的详细信息
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				log.Printf("无法获取块详细信息: %v", err)
				continue
			}

			// 打印块中的交易
			fmt.Printf("块 %d 包含 %d 笔交易:\n", block.NumberU64(), len(block.Transactions()))
			for _, tx := range block.Transactions() {
				printTransactionDetails(tx, client, block)
			}
		}

		time.Sleep(10 * time.Second) // 每10秒检查一次新区块
	}
}

func printTransactionDetails(tx *types.Transaction, client *ethclient.Client, block *types.Block) {
	txHash := tx.Hash().Hex()
	txValue := tx.Value() // 获取交易金额
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		log.Printf("无法获取交易的发送者: %v", err)
		return
	}

	// 打印分隔符，明确区分每笔交易
	fmt.Println("--------------------------------------------------")
	fmt.Printf("  交易哈希: %s\n", txHash)
	fmt.Printf("  发送者: %s\n", sender.Hex())
	fmt.Printf("  接收者: %s\n", tx.To().Hex())
	fmt.Printf("  交易金额: %s ETH\n", weiToEth(txValue).String())

	// 判断是否为合约交易
	if isContractTransaction(client, tx.To()) {
		fmt.Printf("  合约调用交易\n")
		printContractCallDetails(tx, client)
	}
}

// weiToEth 用于将wei单位转为eth单位
func weiToEth(wei *big.Int) *big.Float {
	ethValue := new(big.Float).SetInt(wei)
	return new(big.Float).Quo(ethValue, big.NewFloat(1e18))
}

// 判断交易接收者是否为合约地址
func isContractTransaction(client *ethclient.Client, address *common.Address) bool {
	if address == nil {
		return false
	}
	code, err := client.CodeAt(context.Background(), *address, nil) // 获取接收者地址的代码
	if err != nil {
		log.Printf("无法获取地址代码: %v", err)
		return false
	}
	return len(code) > 0 // 如果地址代码长度大于0，则为合约地址
}

// 打印合约调用的详细信息
func printContractCallDetails(tx *types.Transaction, client *ethclient.Client) {
	recipient := tx.To()
	if recipient == nil {
		log.Printf("交易没有接收者，可能是合约创建交易")
		return
	}

	callData := tx.Data()
	if len(callData) == 0 {
		fmt.Println("  无合约调用数据")
		return
	}

	fmt.Printf("  合约地址: %s\n", recipient.Hex())
	fmt.Printf("  调用数据: %x\n", callData)

	// 解析方法ID
	methodID := callData[:4]
	fmt.Printf("  方法ID: %x\n", methodID)

	// 获取合约ABI
	contractABI, err := getContractABI(recipient.Hex(), "2DTB79CHTEJ6PEDCTEINC8GV3IHUXHGP9A") // 使用你自己的API Key
	if err != nil {
		log.Printf("无法获取合约ABI: %v", err)
		return
	}

	// 如果 ABI 获取失败或合约未验证，跳过处理
	if contractABI == "" {
		log.Printf("合约未验证，跳过ABI解析")
		return
	}

	// 解析合约ABI
	parsedABI, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		log.Printf("无法解析合约ABI: %v", err)
		return
	}

	// 获取方法
	method, err := parsedABI.MethodById(methodID)
	if err != nil {
		log.Printf("无法识别合约方法: %v", err)
		return
	}

	fmt.Printf("  调用方法: %s\n", method.Name)

	// 解析输入参数
	inputs, err := method.Inputs.Unpack(callData[4:])
	if err != nil {
		log.Printf("无法解码方法参数: %v", err)
		return
	}

	// 打印每个参数的类型和值
	for i, input := range inputs {
		parameterType := method.Inputs[i].Type.String()
		parameterValue := input
		fmt.Printf("  参数 %d (%s): %v\n", i+1, parameterType, parameterValue)
	}
}

// 获取合约ABI，并限制API调用频率
func getContractABI(contractAddress, apiKey string) (string, error) {
	url := fmt.Sprintf("https://api.etherscan.io/api?module=contract&action=getabi&address=%s&apikey=%s", contractAddress, apiKey)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("获取ABI时出错: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 控制API调用频率：每次请求后休眠200ms，确保每秒最多调用5次
	time.Sleep(200 * time.Millisecond)

	// 解析返回的JSON并提取ABI字段
	type etherscanAPIResponse struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Result  string `json:"result"`
	}
	var result etherscanAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析Etherscan响应失败: %v", err)
	}

	// 如果返回结果为 "Contract source code not verified"，跳过并返回空ABI
	if result.Status != "1" || result.Message == "NOTOK" {
		log.Printf("Etherscan API 错误: 合约源码未验证，跳过 ABI 获取")
		return "", nil // 返回空字符串表示ABI未获取
	}

	return result.Result, nil
}
