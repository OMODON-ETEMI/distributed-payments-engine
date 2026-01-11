import { Request, Response, NextFunction } from 'express';
import { UserService } from './user.service';

interface KarmaResponse {
    status: string;
    message: string;
    data: any; 
}

export class UserController {
    constructor(private userService: UserService) {}

    register = async (req: Request, res: Response, next: NextFunction) => {
        try {
            const { first_name, last_name, email, phone_number, password } = req.body;
            const karmaResponse = await fetch(`${process.env.KARMA_API_BASE_URL}/${email || phone_number}`,{
                method: 'GET',
                headers: {
                    'Authorization': `Bearer ${process.env.KARMA_API_KEY}`,
                    'Content-Type': 'application/json'
                }
            })

            if (karmaResponse.status !== 404) {

                if (karmaResponse.ok) {
                    const errorData = await karmaResponse.json() as KarmaResponse;
                    

                    if (errorData.data && errorData.status === 'success') {
                        return res.status(403).json({
                            status: 'error',
                            message: 'User is blacklisted and cannot be onboarded.',
                            reason: errorData.message 
                        });
                    }
                }
        }
            const result = await this.userService.createUser({
                first_name,
                last_name,
                email,
                phone_number,
                password
            });

            res.status(201).json({
                status: 'success',
                data: {
                    user: {
                        id: result.user.id,
                        email: result.user.email,
                        first_name: result.user.first_name,
                        last_name: result.user.last_name
                    },
                    token: result.token
                }
            });
        } catch (error) {
            next(error);
        }
    };
}