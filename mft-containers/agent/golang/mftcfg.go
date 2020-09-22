/*
© Copyright IBM Corporation 2017, 2020
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"bytes"
	"os/signal"
	"syscall"
	"time"
	"bufio"
    "io/ioutil"
	"container/list"
    "github.com/tidwall/gjson"
	"strconv"
	"errors"
)

// Main entry point to program.
func main () {
  var bfgDataPath string
  var bfgConfigFilePath string 
  var agentConfig string 
  var e error
  var showAgentLogs bool
  var displayLines int64
  var monitorWaitInterval int64
  // Variables for Stdout and Stderr
  var outb, errb bytes.Buffer
  var startOnly bool
  
  // 1- AGENT_CONFIG_FILE
  // 2- START_ONLY
  if len(os.Args) >= 2 {
	// Configuration file path from environment variable
    bfgConfigFilePath = os.Args[1]
	// Read agent configuration data from JSON file.
    agentConfig, e = ReadConfigurationDataFromFile(bfgConfigFilePath)
	// Exit if we had any error when reading configuration file
    if e != nil {
      panic(e)
      return
    }
	
    if (len(os.Args) == 3) {
       b, e := strconv.ParseBool(os.Args[2])
       if e != nil {
	 startOnly = false
       } else {
	 startOnly = b
       }
    }
    // BFG_DATA path
    bfgDataPath = gjson.Get(agentConfig, "dataPath").String()
	// Agent liveliness monitoring interval
	monitorWaitInterval = gjson.Get(agentConfig, "monitoringInterval").Int()
	// To display agent logs or not.
	showAgentLogs = gjson.Get(agentConfig, "displayAgentLogs").Bool()
	// Display n number of logs from agent log
	displayLines = gjson.Get(agentConfig, "displayLineCount").Int()
	// Wait before start
  } else {
    fmt.Println("Invalid parameters were provided.\nUsage: docker run --mount type=volume,source=mftdata,target=/mftdata -e AGENT_CONFIG_FILE=\"/mftdata/agentconfigsrc.json\" -e START_ONLY -d --name=AGENTSRC mftagentredist\n")
  }
  
  // Set BFG_DATA environment variable so that we can run MFT commands.
  os.Setenv("BFG_DATA", bfgDataPath)

  // Get the path of MFT fteSetupCoordination command. 
  cmdCoordPath, lookErr := exec.LookPath("fteSetupCoordination")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  // Get the path of MFT fteSetupCommands command. 
  cmdCmdsPath, lookErr := exec.LookPath("fteSetupCommands")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  // Get the path of MFT fteCreateAgent command. 
  cmdCrtAgntPath, lookErr:= exec.LookPath("fteCreateAgent")
  if lookErr != nil {
	panic(lookErr)
	return
  }

  // Get the path of MFT fteCreateBridgeAgent command
  cmdCrtBridgeAgntPath, lookErr :=exec.LookPath("fteCreateBridgeAgent")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  // Get the path of MFT fteStartAgent command. 
  cmdStrAgntPath, lookErr:= exec.LookPath("fteStartAgent")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  // Get the path of MFT fteStopAgent command. 
  cmdStopAgntPath, lookErr:= exec.LookPath("fteStopAgent")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  // Get the path of MFT ftePingAgent command. 
  cmdPingAgntPath, lookErr:= exec.LookPath("ftePingAgent")
  if lookErr != nil {
    panic(lookErr)
    return
  }

  if !startOnly {
  // Setup coordination configuration
  fmt.Printf("Setting up coordination configuration %s for agent %s\n", gjson.Get(agentConfig,"coordinationQMgr.name"), gjson.Get(agentConfig,"agent.name"))
  cmdSetupCoord := &exec.Cmd {
	Path: cmdCoordPath,
	Args: [] string {cmdCoordPath, "-coordinationQMgr", gjson.Get(agentConfig,"coordinationQMgr.name").String(), 
	                               "-coordinationQMgrHost", gjson.Get(agentConfig,"coordinationQMgr.host").String(), 
	                               "-coordinationQMgrPort",gjson.Get(agentConfig,"coordinationQMgr.port").String(), 
								   "-coordinationQMgrChannel", gjson.Get(agentConfig,"coordinationQMgr.channel").String(), "-f"},
  }

  // Execute the fteSetupCoordination command. Log an error an exit in case of any error.
  cmdSetupCoord.Stdout = &outb
  cmdSetupCoord.Stderr = &errb
  if err := cmdSetupCoord.Run(); err != nil {
	fmt.Println("fteSetupCoordination command failed. The error is: ", err);
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	return
  }

  // Setup commands configuration
  fmt.Printf("Setting up commands configuration %s for agent %s\n", gjson.Get(agentConfig,"coordinationQMgr.name"), gjson.Get(agentConfig,"agent.name"))
  cmdSetupCmds := &exec.Cmd {
	Path: cmdCmdsPath,
	Args: [] string {cmdCmdsPath, "-p", gjson.Get(agentConfig,"coordinationQMgr.name").String(), 
	                              "-connectionQMgr", gjson.Get(agentConfig,"commandsQMgr.name").String(), 
								  "-connectionQMgrHost", gjson.Get(agentConfig,"commandsQMgr.host").String(), 
	                              "-connectionQMgrPort", gjson.Get(agentConfig,"commandsQMgr.port").String(), 
								  "-connectionQMgrChannel", gjson.Get(agentConfig,"commandsQMgr.channel").String(),"-f"},
  }
  
  // Reuse the same buffer
  outb.Reset()
  errb.Reset()
  cmdSetupCmds.Stdout = &outb
  cmdSetupCmds.Stderr = &errb
  // Execute the fteSetupCommands command. Log an error an exit in case of any error.
  if err := cmdSetupCmds.Run(); err != nil {
	fmt.Println("fteSetupCommands command failed. The errror is: ", err);
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	return
  }

  // Create agent.
  fmt.Printf("Creating %s agent with name %s \n", gjson.Get(agentConfig, "agent.type"), gjson.Get(agentConfig, "agent.name"))
  var cmdCrtAgnt * exec.Cmd
  if strings.EqualFold(gjson.Get(agentConfig, "agent.type").String(), "STANDARD") == true {
    cmdCrtAgnt = &exec.Cmd {
	Path: cmdCrtAgntPath,
	Args: [] string {cmdCrtAgntPath, "-p", gjson.Get(agentConfig,"coordinationQMgr.name").String(), 
	                                 "-agentName", gjson.Get(agentConfig,"agent.name").String(), 
									 "-agentQMgr", gjson.Get(agentConfig,"agent.qmgrName").String(), 
	                                 "-agentQMgrHost", gjson.Get(agentConfig,"agent.qmgrHost").String(), 
									 "-agentQMgrPort", gjson.Get(agentConfig,"agent.qmgrPort").String(), 
									 "-agentQMgrChannel", gjson.Get(agentConfig,"agent.qmgrChannel").String(),
									 "-credentialsFile",gjson.Get(agentConfig,"agent.credentialsFile").String(), "-f"},
    }
  } else {
    var  params [] string 
    params = append(params,cmdCrtBridgeAgntPath,  
	                                 "-p", gjson.Get(agentConfig,"coordinationQMgr.name").String(), 
	                                 "-agentName", gjson.Get(agentConfig,"agent.name").String(), 
									 "-agentQMgr", gjson.Get(agentConfig,"agent.qmgrName").String(), 
	                                 "-agentQMgrHost", gjson.Get(agentConfig,"agent.qmgrHost").String(), 
									 "-agentQMgrPort", gjson.Get(agentConfig,"agent.qmgrPort").String(), 
									 "-agentQMgrChannel", gjson.Get(agentConfig,"agent.qmgrChannel").String(),
									 "-credentialsFile",gjson.Get(agentConfig,"agent.credentialsFile").String(), "-f")

	serverType := gjson.Get(agentConfig, "agent.protocolBridge.serverType")
    if serverType.Exists() {
	  params = append(params,"-bt", serverType.String())
    } else {
	  params = append(params, "-bt", "FTP")
    }

	serverHost := gjson.Get(agentConfig, "agent.protocolBridge.serverHost")
    if serverHost.Exists(){
	  params = append(params,"-bh", serverHost.String())
    } else {
	  params = append(params,"-bh", "localhost")
    }

	serverTimezone := gjson.Get(agentConfig, "agent.protocolBridge.serverTimezone")
    if serverTimezone.Exists() {
	  params = append(params,"-btz", serverTimezone.String())
    }

	serverPlatform := gjson.Get(agentConfig, "agent.protocolBridge.serverPlatform")
    if serverPlatform.Exists() {
	  params = append(params,"-bm", serverPlatform.String())
    }

	serverLocale := gjson.Get(agentConfig,"agent.protocolBridge.serverLocale")
    if serverType.String() != "SFTP" &&  serverLocale.Exists() {
	  params = append(params,"-bsl", serverLocale.String())
    }

	serverFileEncoding := gjson.Get(agentConfig,"agent.protocolBridge.serverFileEncoding")
    if serverFileEncoding.Exists() {
	  params = append(params,"-bfe", serverFileEncoding.String())
    }

	serverPort := gjson.Get(agentConfig,"agent.protocolBridge.serverPort")
    if serverPort.Exists() {
	  params = append(params,"-bp", serverPort.String())
    }

    serverTrustStoreFile := gjson.Get(agentConfig,"agent.protocolBridge.serverTrustStoreFile")
    if serverTrustStoreFile.Exists () {
	  params = append(params,"-bts", serverTrustStoreFile.String())
    }

    serverLimitedWrite := gjson.Get(agentConfig,"agent.protocolBridge.serverLimitedWrite")
    if serverLimitedWrite.Exists () {
	  params = append(params,"-blw", serverLimitedWrite.String())
    }

    serverListFormat := gjson.Get(agentConfig,"agent.protocolBridge.serverListFormat")
    if serverListFormat.Exists () {
	  params = append(params,"-blf", serverListFormat.String())
    }

    cmdCrtAgnt = &exec.Cmd {
        Path: cmdCrtBridgeAgntPath,
        Args: params,
    }
  }

  // Reuse the same buffer
  outb.Reset()
  errb.Reset()
  cmdCrtAgnt.Stdout = &outb
  cmdCrtAgnt.Stderr = &errb
  // Execute the fteCreateAgent command. Log an error an exit in case of any error.
  if err := cmdCrtAgnt.Run(); err != nil {
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	fmt.Println("Create Agent command failed. The error is: ", err);
	return
    }
  } // Not Start Only
  
  // Update agent properties file
  agentPropertiesFile := bfgDataPath + "/mqft/config/" + gjson.Get(agentConfig, "coordinationQMgr.name").String() + "/agents/" + gjson.Get(agentConfig,"agent.name").String() + "/agent.properties"
  updateAgentProperties(agentPropertiesFile, agentConfig);
  
  fmt.Printf("Starting agent %s\n", gjson.Get(agentConfig, "agent.name"))
  cmdStrAgnt := &exec.Cmd {
	Path: cmdStrAgntPath,
	Args: [] string {cmdStrAgntPath,"-p", gjson.Get(agentConfig,"coordinationQMgr.name").String(), gjson.Get(agentConfig,"agent.name").String()},
  }
  
  // Reuse the same buffer
  outb.Reset()
  errb.Reset()
  cmdStrAgnt.Stdout = &outb
  cmdStrAgnt.Stderr = &errb
  // Run fteStartAgent command. Log an error and exit in case of any error.
  if err := cmdStrAgnt.Run(); err != nil {
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	return
  }

  fmt.Printf("Verifying status of agent %s\n", gjson.Get(agentConfig, "agent.name"))
  cmdListAgentPath, lookErr := exec.LookPath("fteListAgents")
  if lookErr != nil {
    panic(lookErr)
    return
  }
 
  // Prepare fteListAgents command for execution
  cmdListAgents := &exec.Cmd {
	Path: cmdListAgentPath,
	Args: [] string {cmdListAgentPath, "-p", gjson.Get(agentConfig, "coordinationQMgr.name").String(), gjson.Get(agentConfig,"agent.name").String()},
  }

  // Reuse the same buffer
  outb.Reset()
  errb.Reset()
  cmdListAgents.Stdout = &outb
  cmdListAgents.Stderr = &errb
  // Execute and get the output of the command into a byte buffer.
  err := cmdListAgents.Run()
  if err != nil {
	fmt.Println("Error: ", err)
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	return
  }

  // Now parse the output of fteListAgents command and take appropriate actions.
  var agentStatus string 
  agentStatus = outb.String()

  // Create a go routine to read the agent output0.log file
  if showAgentLogs == true {
    agentLogPath := bfgDataPath + "/mqft/logs/" + gjson.Get(agentConfig, "coordinationQMgr.name").String() + "/agents/" + gjson.Get(agentConfig,"agent.name").String() + "/logs/output0.log"
    DisplayAgentOutputLog(displayLines, agentLogPath)
  }
 
  if strings.Contains(agentStatus,"STOPPED") == true {
    //if agent not started yet, wait for some time and then reissue fteListAgents commad
    fmt.Println("Agent not started yet. Wait for 10 seconds and recheck status again")
    time.Sleep(10 * time.Second)
	
    // Prepare fteListAgents command for execution
    cmdListAgents := &exec.Cmd {
	  Path: cmdListAgentPath,
	  Args: [] string {cmdListAgentPath, "-p", gjson.Get(agentConfig, "coordinationQMgr.name").String(), gjson.Get(agentConfig, "agent.name").String()},
    }
    
	// Execute and get the output of the command into a byte buffer.
    outb.Reset()
    errb.Reset()
    cmdListAgents.Stdout = &outb
    cmdListAgents.Stderr = &errb
    err := cmdListAgents.Run()
    if err != nil {
      fmt.Println("Error: ", err)
	  fmt.Println("Command: %s\n", outb.String())
      fmt.Println("Error %s\n", errb.String())
      return
    }
    // Copy the latest status again.	
	agentStatus = outb.String()
  } // If agent stopped

  // If agent status is READY or ACTIVE, then we are good. 
  if (strings.Contains(agentStatus,"READY") == true ||
     strings.Contains(agentStatus,"ACTIVE") == true)  {
	fmt.Println("Agent has started.")
	 // Create resource monitor if asked for
	 if !startOnly {
	   if gjson.Get(agentConfig, "agent.resourceMonitors").Exists() {
	     result := gjson.Get(agentConfig, "agent.resourceMonitors")
         result.ForEach(func(key, value gjson.Result) bool {
	     createResourceMonitor(gjson.Get(agentConfig, "coordinationQMgr.name").String(), 
	                        gjson.Get(agentConfig, "agent.name").String(),
                            gjson.Get(agentConfig, "agent.qmgrName").String(), 							 
							key.String(), 
							value.String())
	     return true // keep looping till end
      })
     }
    }
	
	// Agent is READY, so start monitoring the status. If the status becomes unknown, 
	// this monitoring program terminates thus container also ends.
	fmt.Println("Starting to monitor agent status")
    
	// Setup channel to handle signals to stop agent
	sigs := make(chan os.Signal, 1)
    
	// Notify monitor program when SIGINT or SIGTERM is issued to container.
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	
    // Handler for receiving singals 
    go func() {
      <-sigs
      fmt.Printf("Stopping agent %s\n", gjson.Get(agentConfig,"agent.name"))
	  cmdStopAgnt := &exec.Cmd {
	    Path: cmdStopAgntPath,
	    Args: [] string {cmdStopAgntPath,"-p", gjson.Get(agentConfig, "coordinationQMgr.name").String(), gjson.Get(agentConfig, "agent.name").String(), "-i"},
	  }
        
      outb.Reset()
      errb.Reset()
      cmdStopAgnt.Stdout = &outb
      cmdStopAgnt.Stderr = &errb
      err := cmdStopAgnt.Run()
      if err != nil {
	    fmt.Println("An error occured when running fteStopAgent command. The error is: ", err)
      }
      fmt.Printf("Stopped agent %s\n", gjson.Get(agentConfig,"agent.name"))
    }()

	// Loop for ever or till asked to stop
	for {
      // Keep running fteListAgents at specified interval.
	  cmdListAgents := &exec.Cmd {
        Path: cmdListAgentPath,
        Args: [] string {cmdListAgentPath, "-p", gjson.Get(agentConfig, "coordinationQMgr.name").String(), gjson.Get(agentConfig, "agent.name").String()},
      }

      outb.Reset()
      errb.Reset()
	  cmdListAgents.Stdout = &outb
      cmdListAgents.Stderr = &errb
      err := cmdListAgents.Run()
      if err != nil {
        fmt.Printf("An error occurred when running fteListAgents command. The error is %s\n: ", err)
	    fmt.Println("Command: %s\n", outb.String())
        fmt.Println("Error %s\n", errb.String())
        return
      }

      // Check if the status of agent is UNKNOWN. If it is run ftePingAgent
	  // to see if the agent is responding. If does not, then stop container.
      var agentStatus string
      agentStatus = outb.String()  
      if strings.Contains(agentStatus,"UNKNOWN") {
        fmt.Println("Agent status unknown. Pinging the agent")
        cmdPingAgent := &exec.Cmd {
	      Path: cmdPingAgntPath,
	      Args: [] string {cmdPingAgntPath, "-p", gjson.Get(agentConfig, "commandsQMgr.name").String(), gjson.Get(agentConfig, "agent.name").String()},
        }
 
        outb.Reset()
        errb.Reset()
	    cmdPingAgent.Stdout = &outb
        cmdPingAgent.Stderr = &errb
	    err := cmdPingAgent.Run()
          if err != nil {
            fmt.Println("An error occurred when running ftePingAgent command. The error is: ", err)
	        fmt.Println("Command: %s\n", outb.String())
            fmt.Println("Error %s\n", errb.String())
            return
          } 

	    var pingStatus string
	    pingStatus = outb.String()
	    if strings.Contains(pingStatus, "BFGCL0214I") {
	      fmt.Printf("Agent %s did not respond to ping. Monitor exiting\n", gjson.Get(agentConfig, "agent.name"))
	      return
	    }
      } else {
	    fmt.Printf("Agent %s is running\n",  gjson.Get(agentConfig, "agent.name"))
	    // Agent is alive, Then sleep for specified time
	    time.Sleep(time.Duration(monitorWaitInterval) * time.Second)
      }
    } // For loop.
  } else {
    fmt.Println("Agent not started. Quitting")
    return
  }
}

// Method to display agent logs from output0.log file
func DisplayAgentOutputLog(displayLines int64, agentLogPath string) {
  // A channel to display logs continuosly.
  go func() {
    f, err := os.Open(agentLogPath)
	defer f.Close()
    if err != nil {
      fmt.Printf("error opening file: %v\n",err)
      return
    }

	fmt.Println("=======================================================================")
	fmt.Println("============================= Agent logs ==============================")
	fmt.Println("=======================================================================")
    logFileLines := list.New()
    r := bufio.NewReader(f)
    for {
      s, e := Readln(r)
	  if e == nil {
		logFileLines.PushBack(s)
	    if int64(logFileLines.Len()) == displayLines {
		  element := logFileLines.Front()
		  logFileLines.Remove(element)
	    }
	  } else {
		break
	  }
    }
      
	for element := logFileLines.Front(); element != nil; element = element.Next() {
	  fmt.Println(element.Value)
    }
	  
	for {
      s, e := Readln(r)
      for e == nil {
        fmt.Println(s)
        s,e = Readln(r)
      }
	}
  }()
}

// Method to read a line from agents output0.log file
func Readln(r *bufio.Reader) (string, error) {
  var (isPrefix bool = true
       err error = nil
       line, ln []byte
      )
  for isPrefix && err == nil {
      line, isPrefix, err = r.ReadLine()
      ln = append(ln, line...)
  }
  return string(ln),err
}

// Read configuration data from json file
func ReadConfigurationDataFromFile(configFile string) (string, error ) {
  var agentConfig string
  jsonFile, err := os.Open(configFile)
  // if we os.Open returns an error then handle it
  if err != nil {
    fmt.Println(err)
	return agentConfig, err
  }
  
  fmt.Println("Setting up agent using configuration file " + configFile)
  // defer the closing of our jsonFile so that we can parse it later on
  defer jsonFile.Close()
  
  // read file
  var data []byte
  data, err = ioutil.ReadAll(jsonFile)
  if err != nil {
     fmt.Print(err)
	 return agentConfig, err
  }
  agentConfig = string(data)
  
  err = validateAttributes(agentConfig)
  return agentConfig, err
}

// Validate attributes in JSON file.
func validateAttributes(jsonData string) (error){
  // Datapath is a mandatory attribute.
  if !gjson.Get(jsonData, "dataPath").Exists() {
	err := errors.New("dataPath attribute missing. Can't setup agent configuration")
	return err
  }
  
  // Coordination queue manager is mandatory
  if !gjson.Get(jsonData, "coordinationQMgr.name").Exists() {
	err := errors.New("Coordination queue manager name missing. Can't setup agent configuration")
	return err
  }

  // Commands queue manager is mandatory
  if !gjson.Get(jsonData, "commandsQMgr.name").Exists() {
	err := errors.New("Command queue manager name missing. Can't setup agent configuration")
	return err
  }

  // Agent name is mandatory
  if !gjson.Get(jsonData, "agent.name").Exists() {
	err := errors.New("Agent name missing. Can't setup agent configuration")
	return err
  }

  // Agent queue manager name is mandatory
  if !gjson.Get(jsonData, "agent.qmgrName").Exists() {
	err := errors.New("Agent queue manager name missing. Can't setup agent configuration")
	return err
  }

  // Agent queue manager name is mandatory
  if !gjson.Get(jsonData, "agent.credentialsFile").Exists() {
	err := errors.New("Agent credentials file missing. Can't setup agent configuration")
	return err
  }
  return nil
}

// Create resource monitor 
func createResourceMonitor(coordinationQMgr string, agentName string, agentQMgr string, monitorName string, fileName string) (error) {
  var outb, errb bytes.Buffer
  fmt.Println("Creating resource monitor " + monitorName)
  
  // Get the path of MFT fteCreateAgent command. 
  cmdCrtMonitorPath, lookErr:= exec.LookPath("fteCreateMonitor")
  if lookErr != nil {
	return lookErr
  }
  cmdCrtMonitorCmd := &exec.Cmd {
	Path: cmdCrtMonitorPath,
	Args: [] string {cmdCrtMonitorPath, "-p", coordinationQMgr, 
	                              "-mm", agentQMgr, 
								  "-ma", agentName, 
	                              "-mn", monitorName, 
								  "-ix", fileName,"-f"},
  }

  // Reuse the same buffer
  cmdCrtMonitorCmd.Stdout = &outb
  cmdCrtMonitorCmd.Stderr = &errb
  // Execute the fteSetupCommands command. Log an error an exit in case of any error.
  if err := cmdCrtMonitorCmd.Run(); err != nil {
	fmt.Println("fteCreateMonitor command failed. The errror is: ", err);
	fmt.Println("Command: %s\n", outb.String())
    fmt.Println("Error %s\n", errb.String())
	return err
  }
  return nil
}

func updateAgentProperties(propertiesFile string, agentConfig string) {
  f, err := os.OpenFile(propertiesFile, os.O_APPEND|os.O_WRONLY, 0644)
  if err != nil {
	fmt.Println(err)
  }
  defer f.Close()
  
  if gjson.Get(agentConfig, "agent.additionalProperties").Exists() {
	result := gjson.Get(agentConfig, "agent.additionalProperties")
    result.ForEach(func(key, value gjson.Result) bool {
       if _, err := f.WriteString("\n" + key.String() + "=" + value.String() + "\n"); err != nil {
	     fmt.Println(err)
       }
	   return true // keep iterating
      })
  }
}