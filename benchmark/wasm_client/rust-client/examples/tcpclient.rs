use rust_client::client::HydisTcpClient;
use serde::{Deserialize, Serialize};
use serde_json;
use std::collections::HashMap;
use std::io::{Read, Write};
use wasmedge_wasi_socket::{Shutdown, TcpStream};

#[derive(Serialize)]
struct Message {
    consistency: String,
    operation: String,
    key: String,
    value: String,
    vector_clock: HashMap<String, u64>,
}

#[allow(dead_code)]
// 在单个Tcp连接下，发送总计times次请求，并且put比例为ratio
fn put_get_multitimes(times: i32, ratio: f32) -> std::io::Result<()> {
    let kvs = format!("192.168.10.120:50000");
    // 创建客户端
    let mut client = HydisTcpClient::new();
    let mut vc_map = HashMap::new();
    vc_map.insert("192.168.10.120:30881".to_string(), 0);
    client.set_vc(vc_map);
    println!("test!");
    // 创建消息体，并序列化为JSON
    /* let put_causal_message = Message {
        consistency: "PutInCausal".to_string(),
        operation: "Put".to_string(),
        key: "k1".to_string(),
        value: "v1".to_string(),
        vector_clock: client.get_vc(),
    };

    let get_causal_message = Message {
        consistency: "GetInCausal".to_string(),
        operation: "Get".to_string(),
        key: "k2".to_string(),
        value: "".to_string(),
        vector_clock: client.get_vc(),
    };

    let json = serde_json::to_string(&get_causal_message).unwrap();

    let mut stream = TcpStream::connect(kvs)?;
    stream.set_nonblocking(true)?;
    stream.write(json.as_bytes())?;
    // 持续等待响应
    loop {
        let mut buf = [0; 128];
        match stream.read(&mut buf) {
            Ok(0) => {
                println!("server closed connection");
                break;
            }
            Ok(size) => {
                let buf = &mut buf[..size];
                println!("response: {}", String::from_utf8_lossy(buf));
                break;
            }
            Err(e) => {
                // WouldBlock: The operation needs to block to complete, but the blocking operation was requested to not occur.
                // 无视这个错误，继续循环
                if e.kind() == std::io::ErrorKind::WouldBlock {
                } else {
                    return Err(e);
                }
            }
        };
    }
    stream.shutdown(Shutdown::Both)?;
    Ok(()) */

    let mut stream = TcpStream::connect(kvs)?;
    let put_times = (ratio * 10.0) as i32;
    let get_times = ((1.0 - ratio) * 10.0) as i32;
    for i in 0..(times / 10) {
        for m in 0..put_times {
            let put_causal_message = Message {
                consistency: "PutInCausal".to_string(),
                operation: "Put".to_string(),
                key: "k1".to_string() + &(m + 10 * i).to_string(),
                value: "v1".to_string() + &(m + 10 * i).to_string(),
                vector_clock: client.get_vc(),
            };
            let json = serde_json::to_string(&put_causal_message).unwrap();
            stream.write(json.as_bytes())?;

            // 等待响应
            loop {
                let mut buf = [0; 128];
                match stream.read(&mut buf) {
                    Ok(0) => {
                        println!("server closed connection");
                        break;
                    }
                    Ok(size) => {
                        let buf = &mut buf[..size];
                        println!("response: {}", String::from_utf8_lossy(buf));
                        break;
                    }
                    Err(e) => {
                        if e.kind() == std::io::ErrorKind::WouldBlock {
                        } else {
                            return Err(e);
                        }
                    }
                };
            }
        }
        for _ in 0..get_times {
            let get_causal_message = Message {
                consistency: "GetInCausal".to_string(),
                operation: "Get".to_string(),
                key: "k2".to_string() + &(10 * i).to_string(),
                value: "".to_string(),
                vector_clock: client.get_vc(),
            };
            let json = serde_json::to_string(&get_causal_message).unwrap();
            stream.write(json.as_bytes())?;

            loop {
                let mut buf = [0; 128];
                match stream.read(&mut buf) {
                    Ok(0) => {
                        println!("server closed connection");
                        break;
                    }
                    Ok(size) => {
                        let buf = &mut buf[..size];
                        println!("response: {}", String::from_utf8_lossy(buf));
                        break;
                    }
                    Err(e) => {
                        if e.kind() == std::io::ErrorKind::WouldBlock {
                        } else {
                            return Err(e);
                        }
                    }
                };
            }
        }
    }
    Ok(())
}

/* fn main() -> std::io::Result<()> {
    let kvs = format!("192.168.10.120:50000");
    // 创建客户端
    let mut client = HydisTcpClient::new();
    let mut vc_map = HashMap::new();
    vc_map.insert("192.168.10.120:30881".to_string(), 0);
    client.set_vc(vc_map);
    println!("test!");
    // 创建消息体，并序列化为JSON
    let put_causal_message = Message {
        consistency: "PutInCausal".to_string(),
        operation: "Put".to_string(),
        key: "k1".to_string(),
        value: "v1".to_string(),
        vector_clock: client.get_vc(),
    };

    let get_causal_message = Message {
        consistency: "GetInCausal".to_string(),
        operation: "Get".to_string(),
        key: "k2".to_string(),
        value: "".to_string(),
        vector_clock: client.get_vc(),
    };

    let json = serde_json::to_string(&get_causal_message).unwrap();

    let mut stream = TcpStream::connect(kvs)?;
    stream.set_nonblocking(true)?;
    stream.write(json.as_bytes())?;
    // 持续等待响应
    loop {
        let mut buf = [0; 128];
        match stream.read(&mut buf) {
            Ok(0) => {
                println!("server closed connection");
                break;
            }
            Ok(size) => {
                let buf = &mut buf[..size];
                println!("response: {}", String::from_utf8_lossy(buf));
                break;
            }
            Err(e) => {
                // WouldBlock: The operation needs to block to complete, but the blocking operation was requested to not occur.
                // 无视这个错误，继续循环
                if e.kind() == std::io::ErrorKind::WouldBlock {
                } else {
                    return Err(e);
                }
            }
        };
    }
    stream.shutdown(Shutdown::Both)?;
    Ok(())
}
 */
fn main() -> std::io::Result<()> {
    let start_time = chrono::Utc::now().timestamp_millis();
    let res = put_get_multitimes(100, 0.9);
    let end_time = chrono::Utc::now().timestamp_millis();
    println!(
        "start_time: {}, end_time:{}, 10000 querys takes : {} ms",
        start_time,
        end_time,
        end_time - start_time
    );
    res
}
