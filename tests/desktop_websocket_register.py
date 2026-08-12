#!/usr/bin/env python3
"""Exercise the RustDesk desktop-client RegisterPk flow over WebSocket."""

from __future__ import annotations

import argparse
import base64
import hashlib
import os
import secrets
import socket
import struct
import time


WEBSOCKET_GUID = b"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def encode_varint(value: int) -> bytes:
    encoded = bytearray()
    while value >= 0x80:
        encoded.append((value & 0x7F) | 0x80)
        value >>= 7
    encoded.append(value)
    return bytes(encoded)


def decode_varint(data: bytes, offset: int) -> tuple[int, int]:
    value = 0
    shift = 0
    while offset < len(data) and shift < 70:
        byte = data[offset]
        offset += 1
        value |= (byte & 0x7F) << shift
        if byte < 0x80:
            return value, offset
        shift += 7
    raise ValueError("invalid protobuf varint")


def bytes_field(number: int, value: bytes) -> bytes:
    return encode_varint((number << 3) | 2) + encode_varint(len(value)) + value


def iter_fields(data: bytes):
    offset = 0
    while offset < len(data):
        key, offset = decode_varint(data, offset)
        number = key >> 3
        wire_type = key & 7
        if wire_type == 0:
            value, offset = decode_varint(data, offset)
        elif wire_type == 2:
            length, offset = decode_varint(data, offset)
            end = offset + length
            if end > len(data):
                raise ValueError("truncated protobuf field")
            value = data[offset:end]
            offset = end
        elif wire_type == 1:
            end = offset + 8
            value = data[offset:end]
            offset = end
        elif wire_type == 5:
            end = offset + 4
            value = data[offset:end]
            offset = end
        else:
            raise ValueError(f"unsupported protobuf wire type {wire_type}")
        yield number, wire_type, value


def register_pk_message() -> tuple[str, bytes]:
    peer_id = str(secrets.randbelow(900_000_000) + 100_000_000)
    register_pk = b"".join(
        (
            bytes_field(1, peer_id.encode()),
            bytes_field(2, os.urandom(16)),
            bytes_field(3, os.urandom(32)),
        )
    )
    # hbb.RendezvousMessage.register_pk is field 15.
    return peer_id, bytes_field(15, register_pk)


def register_pk_result(payload: bytes) -> int:
    for number, wire_type, value in iter_fields(payload):
        if number != 16 or wire_type != 2:
            continue
        # hbb.RegisterPkResponse.result is proto3 field 1. OK (zero) is
        # normally omitted from the nested message, so zero is the default.
        result = 0
        for child_number, child_wire_type, child_value in iter_fields(value):
            if child_number == 1 and child_wire_type == 0:
                result = child_value
        return result
    raise ValueError(f"response does not contain RegisterPkResponse: {payload.hex()}")


def read_exact(connection: socket.socket, length: int) -> bytes:
    chunks = bytearray()
    while len(chunks) < length:
        chunk = connection.recv(length - len(chunks))
        if not chunk:
            raise ConnectionError("WebSocket closed unexpectedly")
        chunks.extend(chunk)
    return bytes(chunks)


def send_frame(connection: socket.socket, opcode: int, payload: bytes) -> None:
    mask = os.urandom(4)
    length = len(payload)
    header = bytearray([0x80 | opcode])
    if length < 126:
        header.append(0x80 | length)
    elif length <= 0xFFFF:
        header.append(0x80 | 126)
        header.extend(struct.pack("!H", length))
    else:
        header.append(0x80 | 127)
        header.extend(struct.pack("!Q", length))
    header.extend(mask)
    header.extend(byte ^ mask[index % 4] for index, byte in enumerate(payload))
    connection.sendall(header)


def receive_binary_frame(connection: socket.socket) -> bytes:
    while True:
        first, second = read_exact(connection, 2)
        opcode = first & 0x0F
        masked = bool(second & 0x80)
        length = second & 0x7F
        if length == 126:
            length = struct.unpack("!H", read_exact(connection, 2))[0]
        elif length == 127:
            length = struct.unpack("!Q", read_exact(connection, 8))[0]
        mask = read_exact(connection, 4) if masked else b""
        payload = read_exact(connection, length)
        if masked:
            payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
        if opcode == 0x9:
            send_frame(connection, 0xA, payload)
            continue
        if opcode == 0x8:
            raise ConnectionError(f"server closed WebSocket: {payload!r}")
        if opcode != 0x2:
            raise ValueError(f"expected binary WebSocket frame, got opcode {opcode}")
        return payload


def register_once(host: str, port: int, timeout: float) -> str:
    with socket.create_connection((host, port), timeout=timeout) as connection:
        connection.settimeout(timeout)
        websocket_key = base64.b64encode(os.urandom(16)).decode()
        request = (
            "GET / HTTP/1.1\r\n"
            f"Host: {host}:{port}\r\n"
            "Upgrade: websocket\r\n"
            "Connection: Upgrade\r\n"
            f"Sec-WebSocket-Key: {websocket_key}\r\n"
            "Sec-WebSocket-Version: 13\r\n"
            "\r\n"
        )
        connection.sendall(request.encode("ascii"))

        response = bytearray()
        while b"\r\n\r\n" not in response:
            response.extend(connection.recv(4096))
            if len(response) > 64 * 1024:
                raise ValueError("oversized WebSocket handshake response")
        headers = response.split(b"\r\n\r\n", 1)[0].decode("latin-1")
        if not headers.startswith("HTTP/1.1 101 "):
            raise ConnectionError(f"WebSocket upgrade failed: {headers.splitlines()[0]}")
        header_values = {}
        for line in headers.splitlines()[1:]:
            name, separator, value = line.partition(":")
            if separator:
                header_values[name.strip().lower()] = value.strip()
        expected_accept = base64.b64encode(
            hashlib.sha1(websocket_key.encode("ascii") + WEBSOCKET_GUID).digest()
        ).decode()
        if header_values.get("sec-websocket-accept") != expected_accept:
            raise ConnectionError("invalid Sec-WebSocket-Accept header")

        peer_id, payload = register_pk_message()
        send_frame(connection, 0x2, payload)
        result = register_pk_result(receive_binary_frame(connection))
        if result != 0:
            names = {2: "UUID_MISMATCH", 3: "ID_EXISTS", 4: "TOO_FREQUENT", 5: "INVALID_ID_FORMAT", 6: "NOT_SUPPORT", 7: "SERVER_ERROR"}
            raise RuntimeError(f"RegisterPk failed: {names.get(result, result)}")
        return peer_id


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=21118)
    parser.add_argument("--deadline", type=float, default=45)
    parser.add_argument("--timeout", type=float, default=5)
    args = parser.parse_args()

    deadline = time.monotonic() + args.deadline
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            peer_id = register_once(args.host, args.port, args.timeout)
            print(f"desktop WebSocket RegisterPk succeeded for peer {peer_id}")
            return
        except (ConnectionError, OSError, RuntimeError, ValueError) as error:
            last_error = error
            time.sleep(0.5)
    raise SystemExit(f"desktop WebSocket RegisterPk did not succeed: {last_error}")


if __name__ == "__main__":
    main()
