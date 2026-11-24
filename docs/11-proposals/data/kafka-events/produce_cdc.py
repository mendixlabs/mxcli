"""Publish entity change events the way the Mendix runtime's CDC does.

Envelope copied from duckgraphlake's own KafkaProduceTest.java: the Debezium
envelope, Kafka Connect JSON with the schema embedded. Topic is
cdc.<App>.<Entity>.<Major>.<space>, which is what mxcli's CDC service writes.
"""
import base64, json, struct, sys, time
from confluent_kafka import Producer

# APP is the CDC service document's AppName property, NOT the module the tracked
# entities live in. Measured against a running TestApp (11.16.1): the runtime
# publishes entities of module CdcPublish to cdc.TestApp.*, because the service's
# AppName is "TestApp". This script had "CdcPublish" here and so produced to
# topics no Mendix app writes to -- the shape was right, the name was not, and
# only a runtime could tell the difference.
APP, SPACE, REV = "TestApp", "local", 1

# A Mendix object id carries the entity type in the bits above 48. The server
# resolves an association's target entity by matching those bits, so plain small
# integers make every entity look like the same type and the target comes out
# wrong -- which is a property of the fixture, not of either tool.
# The real codes are nothing like a sequence -- measured on the same app, Order
# was 1 and Customer 60. Any two distinct values exercise the association
# resolution; what breaks it is plain small ids, where every entity shares code 0
# and the target resolves to the wrong entity.
CUSTOMER_TYPE, ORDER_TYPE = 60, 1


def oid(type_code, seq):
    return (type_code << 48) | seq


def dec(value, scale=8):
    unscaled = int(round(value * (10 ** scale)))
    n = (unscaled.bit_length() + 8) // 8 or 1
    return base64.b64encode(unscaled.to_bytes(n, "big", signed=True)).decode()


def fields(spec):
    out = []
    for name, kind in spec:
        if kind == "int64":
            out.append({"field": name, "type": "int64"})
        elif kind == "string":
            out.append({"field": name, "type": "string"})
        elif kind == "boolean":
            out.append({"field": name, "type": "boolean"})
        elif kind == "decimal":
            out.append({"field": name, "name": "org.apache.kafka.connect.data.Decimal",
                        "parameters": {"connect.decimal.precision": "28", "scale": "8"}, "type": "bytes"})
        elif kind == "timestamp":
            out.append({"field": name, "name": "org.apache.kafka.connect.data.Timestamp", "type": "int64"})
        elif kind == "assoc":
            out.append({"field": name, "items": {"type": "int64"}, "type": "array"})
        else:
            raise SystemExit("unknown kind " + kind)
    return out


def change(entity, spec, op, before, after, tx, seq):
    f = fields(spec)
    schema = {"type": "struct", "name": entity, "fields": [
        {"field": "before", "type": "struct", "optional": True, "fields": f},
        {"field": "after", "type": "struct", "optional": True, "fields": f},
        {"field": "op", "type": "string"}]}
    payload = {"before": before, "after": after, "op": op, "ts_ms": 1794940282647,
               "source": {"connector": "mendix_event_broker_cdc", "name": "mendix", "snapshot": False,
                          "sequence": seq, "ts_ms": 1794940282450, "txId": tx, "version": "11.16.1"}}
    return json.dumps({"schema": schema, "payload": payload})


CUSTOMER = [("id", "int64"), ("CustomerNumber", "int64"), ("Name", "string"),
            ("Email", "string"), ("Active", "boolean")]
ORDER = [("id", "int64"), ("OrderNumber", "int64"), ("TotalAmount", "decimal"),
         ("PlacedOn", "timestamp"), ("Order_Customer", "assoc")]


def customer(oid, num, name, email, active=True):
    return {"id": oid, "CustomerNumber": num, "Name": name, "Email": email, "Active": active}


def order(oid, num, amount, cust):
    return {"id": oid, "OrderNumber": num, "TotalAmount": dec(amount),
            "PlacedOn": 1794940282450, "Order_Customer": [cust]}


def main(bootstrap):
    p = Producer({"bootstrap.servers": bootstrap, "compression.type": "none"})
    ct = f"cdc.{APP}.Customer.{REV}.{SPACE}"
    ot = f"cdc.{APP}.Order.{REV}.{SPACE}"
    sent = []

    def send(topic, key, value):
        p.produce(topic, key=str(key), value=value,
                  on_delivery=lambda e, m: sent.append((e, None if e else (m.topic(), m.offset()))))

    for seq, num, name, email in [(7, 1001, "Ada Lovelace", "ada@example.com"),
                                  (8, 1002, "Bob Stone", "bob@example.com"),
                                  (9, 1003, "Cy Vance", "cy@example.com")]:
        cid = oid(CUSTOMER_TYPE, seq)
        send(ct, cid, change("Customer", CUSTOMER, "c", None, customer(cid, num, name, email), "tx-1", 1))
    # one update and one delete, so the current-state view has to do real work
    c8 = oid(CUSTOMER_TYPE, 8)
    send(ct, c8, change("Customer", CUSTOMER, "u", customer(c8, 1002, "Bob Stone", "bob@example.com"),
                       customer(c8, 1002, "Bob Stone", "bob.stone@example.com"), "tx-2", 2))
    c9 = oid(CUSTOMER_TYPE, 9)
    send(ct, c9, change("Customer", CUSTOMER, "d", customer(c9, 1003, "Cy Vance", "cy@example.com"), None, "tx-3", 3))
    for seq, num, amount, cust_seq in [(11, 5001, 249.95, 7), (12, 5002, 19.5, 7), (13, 5003, 1200.0, 8)]:
        o = oid(ORDER_TYPE, seq)
        send(ot, o, change("Order", ORDER, "c", None,
                           order(o, num, amount, oid(CUSTOMER_TYPE, cust_seq)), "tx-4", 4))
    p.flush(20)
    errs = [e for e, _ in sent if e]
    print(f"{len(sent)} delivered, {len(errs)} failed")
    for e in errs:
        print("  error:", e)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1:9092")
