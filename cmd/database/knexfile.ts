
import type { Knex } from 'knex';
import dotenv from "dotenv";

dotenv.config();

const config: { [key: string]: Knex.Config } = {
  production: {
      client: "mysql2",
      connection: process.env.DATABASE_URL, // Railway provides this
      migrations: {
        directory: "./migrations",
        extension: "js",
      },
    },
  development: {
    client: 'mysql2',
    connection: {
      host: process.env.DB_HOST,
      port: Number(process.env.DB_PORT),
      user: process.env.DB_USER,
      password: process.env.DB_PASSWORD,
      database: process.env.DB_NAME,
    },
    migrations: {
      directory: './migrations',
      tableName: 'knex_migrations',
    },
  },
};

export default config;
