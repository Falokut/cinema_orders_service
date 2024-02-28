db = db.getSiblingDB(process.env.MONGO_INITDB_DATABASE);

db.createUser(
    {
        user: process.env.SERVICE_NAME,
        pwd: process.env.SERVICE_PASSWORD,
        roles: [
            {
                role: "readWrite",
                db: process.env.MONGO_INITDB_DATABASE,
            }
        ],
    }
);

collection = db.createCollection("orders");
collection = db.createIndex({ "screening_id": 1 });
collection = db.createIndex({ "status": 1 })
